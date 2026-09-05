package activity

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ActivityServer) StartRefresh(ctx context.Context, req *papi.ActivityStartRefreshRequest) (*papi.ActivityStartRefreshResponse, error) {
	if req == nil || req.GetActivityRefreshId() == "" || req.GetActivityRefreshLevel() <= 0 {
		return nil, viber.InvalidArgumentf("activityRefreshId and a positive activityRefreshLevel are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityStartRefreshResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		info, err := opuser.FindUserRefreshInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		now := time.Now().UnixMilli()
		if err := reconcileRefreshInfo(ctx, suid, now, info); err != nil {
			return nil, err
		}
		if info.GetActiveRefresh() != nil {
			return nil, viber.FailedPreconditionf("a refresh is already active")
		}
		if _, err := opuser.FindUserRefreshRun(ctx, suid); err == nil {
			return nil, viber.FailedPreconditionf("a refresh is already active")
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		refresh := activityRefreshByID(info, req.GetActivityRefreshId())
		if refresh == nil || !refresh.GetIsUnlocked() {
			return nil, viber.FailedPreconditionf("refresh %q is not unlocked", req.GetActivityRefreshId())
		}
		level := activityRefreshLevel(refresh, req.GetActivityRefreshLevel())
		if level == nil || !level.GetIsUnlocked() {
			return nil, viber.FailedPreconditionf("refresh level %d is not unlocked", req.GetActivityRefreshLevel())
		}
		characters, lineup, err := validateActivityLineup(
			ctx, suid, level.GetActivityPerformanceId(), req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds(), now,
		)
		if err != nil {
			return nil, err
		}
		for _, character := range characters {
			if character.GetInActivity() {
				return nil, viber.FailedPreconditionf("character %q is already in an activity", character.GetCharacterId())
			}
		}
		if refresh.GetRequiredMinutes() <= 0 || level.GetRecoveryAmount() <= 0 || level.GetRequiredGold() < 0 {
			return nil, status.Error(codes.FailedPrecondition, "refresh master settings are invalid")
		}
		requiredGold, err := viber.ActivityRefreshGoldCost(level.GetRequiredGold(), info.GetCampaignEffects())
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		if requiredGold > 0 {
			gold, err := viber.ConsumeGold(ctx, suid, requiredGold)
			if err != nil {
				return nil, err
			}
			updated.Points = append(updated.Points, gold)
		}
		duration := int64(refresh.GetRequiredMinutes()) * int64(time.Minute/time.Millisecond)
		if now > math.MaxInt64-duration {
			return nil, status.Error(codes.FailedPrecondition, "refresh finish time overflows")
		}
		finishTime := now + duration
		progress := &papi.RefreshProgress{
			ActivityRefreshId:      refresh.GetId(),
			ActivityRefreshName:    refresh.GetName(),
			ActivityRefreshSubName: level.GetName(),
			ActivityRefreshLevel:   level.GetLevel(),
			Characters:             lineup,
			StartTime:              now,
			FinishTime:             finishTime,
			RequiredMinutes:        refresh.GetRequiredMinutes(),
			RecoveryAmount:         level.GetRecoveryAmount(),
			ActivityPerformanceId:  level.GetActivityPerformanceId(),
			SubGenre:               level.GetSubGenre(),
		}
		for _, character := range characters {
			character.InActivity = true
			if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
				return nil, err
			}
			missions, err := viber.AdvanceUserMissions(
				ctx, suid, penum.MissionType_MissionType_CharacterStartActivityRefreshmentCount, 1, character.GetCharacterId(),
			)
			if err != nil {
				return nil, err
			}
			updated.Missions = append(updated.Missions, missions...)
		}
		updated.Characters = characters
		if err := opuser.UpsertUserRefreshRun(ctx, suid, progress); err != nil {
			return nil, err
		}
		info.ActiveRefresh = &papi.ActiveRefresh{
			CharacterIds: append([]string(nil), req.GetCharacterIds()...),
			Name:         refresh.GetName(),
			SubName:      level.GetName(),
			FinishTime:   finishTime,
		}
		if err := opuser.UpsertUserRefreshInfo(ctx, suid, info); err != nil {
			return nil, err
		}
		response = &papi.ActivityStartRefreshResponse{
			Progress:       progress,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

// reconcileRefreshInfo refreshes the condition-backed refresh gates before a
// direct Start call, which need not be preceded by Home.Enter.
func reconcileRefreshInfo(ctx context.Context, suid string, now int64, info *papi.RefreshInfo) error {
	return viber.ReconcileRefreshInfo(ctx, suid, now, viber.NewCommerceConditionEvaluator(ctx, suid, now), info)
}

func activityRefreshLevel(refresh *papi.Refresh, level int32) *papi.RefreshLevel {
	for _, candidate := range refresh.GetLevels() {
		if candidate.GetLevel() == level {
			return candidate
		}
	}
	return nil
}
