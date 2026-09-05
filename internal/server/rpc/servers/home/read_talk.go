package home

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *HomeServer) ReadTalk(ctx context.Context, req *papi.HomeReadTalkRequest) (*papi.HomeReadTalkResponse, error) {
	if req.GetHomeTalkId() == "" {
		return nil, status.Error(codes.InvalidArgument, "home talk ID is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.HomeReadTalkResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		master, err := opmaster.FindHomeTalk(ctx, req.GetHomeTalkId())
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.NotFound, "home talk does not exist")
			}
			return nil, err
		}
		character, err := opuser.FindUserCharacter(ctx, suid, master.GetCharacterId())
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.PermissionDenied, "home talk character is not owned")
			}
			return nil, err
		}
		_, err = opuser.FindUserHomeTalk(ctx, suid, master.GetHomeTalkId())
		if err == nil {
			response = &papi.HomeReadTalkResponse{CommonResponse: viber.NewCommonResponse(nil)}
			return nil, nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		now := time.Now().UnixMilli()
		available, err := viber.NewCommerceConditionEvaluator(ctx, suid, now).Satisfied(master.GetConditionId())
		if err != nil {
			return nil, err
		}
		if !available || master.GetReleaseDate() > now {
			return nil, status.Error(codes.FailedPrecondition, "home talk is not unlocked")
		}
		if master.GetCardId() != "" {
			if _, err := opuser.FindUserCard(ctx, suid, master.GetCardId()); err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					return nil, status.Error(codes.FailedPrecondition, "home talk card is not owned")
				}
				return nil, err
			}
		}
		reliability := master.GetReliability()
		if reliability < 0 || reliability > math.MaxInt32-character.GetReliability() {
			return nil, status.Error(codes.FailedPrecondition, "invalid home talk reliability reward")
		}
		character.Reliability += reliability
		character.LastHomeTalkFirstReadTime = now
		if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
			return nil, err
		}
		talk := &ptransaction.UserHomeTalk{HomeTalkId: master.GetHomeTalkId()}
		if err := opuser.UpsertUserHomeTalk(ctx, suid, talk); err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{
			Characters: []*ptransaction.UserCharacter{character},
			HomeTalks:  []*ptransaction.UserHomeTalk{talk},
		}
		if reliability > 0 {
			updated.Missions, err = viber.ReachUserMissions(ctx, suid,
				penum.MissionType_MissionType_ReliabilityAchievement, int64(character.GetReliability()), character.GetCharacterId())
			if err != nil {
				return nil, err
			}
			if err := reconcileNarrativeSchedules(ctx, suid, now, updated); err != nil {
				return nil, err
			}
		}
		talkMissions, err := viber.ReachCharacterHomeTalkMissions(ctx, suid, character.GetCharacterId())
		if err != nil {
			return nil, err
		}
		updated.Missions = append(updated.Missions, talkMissions...)
		response = &papi.HomeReadTalkResponse{IsReliabilityUp: reliability > 0, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}
