package dream

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	dreamlogic "github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *DreamServer) StartQuest(ctx context.Context, req *papi.DreamStartQuestRequest) (*papi.DreamStartQuestResponse, error) {
	if req.GetBasePhotoId() == "" || req.GetDeckNumber() <= 0 {
		return nil, viber.InvalidArgumentf("basePhotoId and a positive deckNumber are required")
	}
	if len(req.GetUserBuddyDeckPositions()) > 0 {
		return nil, viber.FailedPreconditionf("friend rental is unavailable in this private server")
	}
	var err error
	ctx, err = mcontext.WithLiveRequest(ctx)
	if err != nil {
		return nil, err
	}
	seed, started, _ := mcontext.GetLiveRequest(ctx)
	now, suid := started.UnixMilli(), viber.GetSuid(ctx).String()
	var response *papi.DreamStartQuestResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		// Serialize with both ordinary lives and Dream mutations. The outer
		// request entropy is reused if a concurrent mutation requires a retry.
		if _, err := opuser.LockUserQuestStart(ctx, suid); err != nil {
			return nil, err
		}
		if err := viber.CheckDreamUnlocked(ctx, suid); err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		if _, err := viber.ReconcileDream(ctx, suid, now, updated); err != nil {
			return nil, err
		}
		area, err := opuser.FindUserDreamArea(ctx, suid, req.GetBasePhotoId())
		if err != nil {
			return nil, err
		}
		photo, err := opuser.FindUserPhoto(ctx, suid, req.GetBasePhotoId())
		if err != nil {
			return nil, err
		}
		if !photo.GetIsDreamBase() || area.Data == nil {
			return nil, viber.FailedPreconditionf("photo has no generated Dream area")
		}
		next := area.Data.GetClearQuestNumber() + 1
		if next <= 0 || next > area.Data.GetMaxQuestNumber() {
			return nil, viber.FailedPreconditionf("all Dream stages are already cleared")
		}
		var stage *papi.DreamQuestInfo
		for _, candidate := range area.Quests {
			if candidate.GetQuestNumber() == next {
				stage = candidate
				break
			}
		}
		if stage == nil {
			return nil, viber.FailedPreconditionf("next Dream stage is unavailable")
		}
		quest := dreamlogic.QuestMaster(stage)
		formation, err := viber.LoadLiveFormation(ctx, suid, req.GetDeckNumber())
		if err != nil {
			return nil, err
		}
		input, err := viber.PrepareLiveInput(ctx, suid, &papi.QuestStartRequest{
			QuestId: quest.GetId(), DeckNumber: req.GetDeckNumber(), PlayCount: 1, IsSkip: req.GetIsSkip(),
		}, quest, seed, formation)
		if err != nil {
			return nil, err
		}
		// isSkip skips playback, not the simulation or its clear requirement.
		result, err := live.Simulate(ctx, input)
		if err != nil {
			var unsupported *live.UnsupportedError
			if errors.As(err, &unsupported) {
				return nil, status.Error(codes.Unimplemented, unsupported.Error())
			}
			return nil, err
		}
		if len(result.GetUserInfos()) == 0 || result.GetUserInfos()[0].GetUserResult() == nil {
			return nil, viber.FailedPreconditionf("live produced no player result")
		}
		// Dream has no replay lookup RPC. The response contains the full trace;
		// a retry-stable opaque ID is enough for existing analysis telemetry.
		result.DetailLogId = uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("dream:%s:%s:%d:%016x:%d", suid, req.GetBasePhotoId(), next, seed, started.UnixNano()))).String()
		reward := &papi.QuestReward{}
		var changes []*papi.DreamUserPhotoAbilityChangeInfo
		if result.GetCleared() {
			changes, err = dreamlogic.ApplyProgress(ctx, suid, area, photo, next, now)
			if err != nil {
				return nil, err
			}
			updated.Photos = append(updated.Photos, photo)
			if err := viber.GrantLiveMastery(ctx, suid, quest.GetMusicId(), viber.LiveResultCharacterIDs(input, result), 1, reward, updated); err != nil {
				return nil, err
			}
			if err := viber.AdvanceDreamMissions(ctx, suid, viber.DreamMissionProgress{Cleared: 1, Difficulty: quest.GetDifficultyLevel()}, now, updated); err != nil {
				return nil, err
			}
		} else {
			area.Data.LastChallengedTime = now
			if err := opuser.UpsertUserDreamArea(ctx, area); err != nil {
				return nil, err
			}
		}
		response = &papi.DreamStartQuestResponse{
			Result: result, ClearedQuestNumber: area.Data.GetClearQuestNumber(), MaxQuestNumber: area.Data.GetMaxQuestNumber(),
			DreamUserPhoto: area.Data, AbilityChangeInfos: changes, MusicMasteryRewards: reward.GetMusicMasteryRewards(),
			RankPatterns: viber.LiveClearRankPatterns(quest.GetClearScore()), CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
