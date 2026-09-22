package dream

import (
	"context"
	"sort"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	dreamlogic "github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *DreamServer) SkipQuest(ctx context.Context, req *papi.DreamSkipQuestRequest) (*papi.DreamSkipQuestResponse, error) {
	if req.GetBasePhotoId() == "" || req.GetDeckNumber() <= 0 || req.GetQuestNumber() <= 0 {
		return nil, viber.InvalidArgumentf("basePhotoId, deckNumber and questNumber are required")
	}
	suid, now := viber.GetSuid(ctx).String(), time.Now().UnixMilli()
	var response *papi.DreamSkipQuestResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
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
		before, target := area.Data.GetClearQuestNumber(), req.GetQuestNumber()
		if !photo.GetIsDreamBase() || area.Data == nil || target <= before || target > area.Data.GetMaxQuestNumber()*4/5 {
			return nil, viber.FailedPreconditionf("only uncleared stages within the first four fifths may be skipped")
		}
		formation, err := viber.LoadLiveFormation(ctx, suid, req.GetDeckNumber())
		if err != nil {
			return nil, err
		}
		if len(formation.Positions) != 5 {
			return nil, viber.FailedPreconditionf("a Dream unit requires five occupied positions")
		}
		seenLanes, seenCards, seenCharacters := map[int32]bool{}, map[string]bool{}, map[string]bool{}
		characters := make([]string, 0, 5)
		for _, position := range formation.Positions {
			if position.GetPosition() < 1 || position.GetPosition() > 5 || seenLanes[position.GetPosition()] || position.GetCardId() == "" || seenCards[position.GetCardId()] {
				return nil, viber.FailedPreconditionf("invalid Dream unit positions")
			}
			seenLanes[position.GetPosition()], seenCards[position.GetCardId()] = true, true
			if _, err := opuser.FindUserCard(ctx, suid, position.GetCardId()); err != nil {
				return nil, err
			}
			card, err := opmaster.FindCard(ctx, position.GetCardId())
			if err != nil {
				return nil, err
			}
			if !seenCharacters[card.GetCharacterId()] {
				if _, err := opuser.FindUserCharacter(ctx, suid, card.GetCharacterId()); err != nil {
					return nil, err
				}
				characters = append(characters, card.GetCharacterId())
				seenCharacters[card.GetCharacterId()] = true
			}
		}
		sort.Strings(characters)
		// Aggregate by song: repeated stages grant one mastery point per unique
		// idol per stage, not per duplicate idol card. No simulation or passes.
		counts := map[string]int32{}
		var musicIDs []string
		var difficulty int32
		var skipped int32
		for _, quest := range area.Quests {
			if quest.GetQuestNumber() <= before || quest.GetQuestNumber() > target {
				continue
			}
			if counts[quest.GetMusicId()] == 0 {
				musicIDs = append(musicIDs, quest.GetMusicId())
			}
			counts[quest.GetMusicId()]++
			difficulty = max(difficulty, quest.GetDifficultyLevel())
			skipped++
		}
		if skipped != target-before {
			return nil, viber.FailedPreconditionf("Dream stages are incomplete")
		}
		rewards := make([]*papi.DreamSkipMusicMasteryRewardInfo, 0, len(musicIDs))
		for _, musicID := range musicIDs {
			questReward, stones := &papi.QuestReward{}, map[string]int64{}
			if err := viber.GrantLiveMastery(ctx, suid, musicID, characters, counts[musicID], questReward, updated, stones); err != nil {
				return nil, err
			}
			row := &papi.DreamSkipMusicMasteryRewardInfo{MusicId: musicID}
			for _, id := range characters {
				if stones[id] > 0 {
					row.CharacterIds = append(row.CharacterIds, id)
					row.TotalStoneAmount += stones[id]
				}
			}
			rewards = append(rewards, row)
		}
		changes, err := dreamlogic.ApplyProgress(ctx, suid, area, photo, target, now)
		if err != nil {
			return nil, err
		}
		updated.Photos = append(updated.Photos, photo)
		if err := viber.AdvanceDreamMissions(ctx, suid, viber.DreamMissionProgress{Cleared: int64(skipped), Difficulty: difficulty}, now, updated); err != nil {
			return nil, err
		}
		response = &papi.DreamSkipQuestResponse{
			ClearedQuestNumber: target, MaxQuestNumber: area.Data.GetMaxQuestNumber(), SkipMusicMasteryRewardInfos: rewards,
			AbilityChangeInfos: changes, DreamUserPhoto: area.Data, CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}
