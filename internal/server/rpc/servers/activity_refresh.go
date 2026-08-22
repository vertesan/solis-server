package servers

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
	"github.com/vertesan/solis-server/internal/utils/config"
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
		if err := viber.ReconcileRefreshInfo(ctx, suid, now, info); err != nil {
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
		characters, lineup, err := viber.ValidateActivityLineup(
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

func (s *ActivityServer) LoadRefresh(ctx context.Context, _ *papi.Empty) (*papi.ActivityLoadRefreshResponse, error) {
	progress, err := opuser.FindUserRefreshRun(ctx, viber.GetSuid(ctx).String())
	if err != nil {
		return nil, viber.GameplayError(activityRunError("refresh", err))
	}
	return &papi.ActivityLoadRefreshResponse{
		Progress:       progress,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *ActivityServer) UseRefreshItem(ctx context.Context, req *papi.ActivityUseRefreshItemRequest) (*papi.ActivityUseRefreshItemResponse, error) {
	if req == nil || req.GetItemId() == "" || req.GetAmount() <= 0 {
		return nil, viber.InvalidArgumentf("a positive refresh item amount is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityUseRefreshItemResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserRefreshRun(ctx, suid)
		if err != nil {
			return nil, activityRunError("refresh", err)
		}
		item, err := opmaster.FindItem(ctx, req.GetItemId())
		if err != nil {
			return nil, err
		}
		if item.GetType() != penum.ItemType_ItemType_ActivityRefreshShorten {
			return nil, viber.InvalidArgumentf("item %q cannot shorten refreshes", req.GetItemId())
		}
		if item.GetValue() <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "refresh item has no shortening value")
		}
		updated := &papi.UpdatedData{}
		if _, err := viber.ConsumeExchangeItemStacks(ctx, suid, req.GetItemId(), int64(req.GetAmount()), updated); err != nil {
			if errors.Is(err, viber.ErrExchangeInsufficientResource) {
				return nil, status.Error(codes.ResourceExhausted, "refresh shortening item is insufficient")
			}
			return nil, err
		}
		shortened, err := activityShorteningMillis(item.GetValue(), req.GetAmount())
		if err != nil {
			return nil, err
		}
		if progress.GetStartTime() < math.MinInt64+shortened || progress.GetFinishTime() < math.MinInt64+shortened {
			return nil, status.Error(codes.FailedPrecondition, "refresh shortening overflows")
		}
		progress.StartTime -= shortened
		progress.FinishTime -= shortened
		if err := opuser.UpsertUserRefreshRun(ctx, suid, progress); err != nil {
			return nil, err
		}
		info, err := opuser.FindUserRefreshInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		if info.ActiveRefresh == nil {
			return nil, status.Error(codes.FailedPrecondition, "active refresh summary is missing")
		}
		info.ActiveRefresh.FinishTime = progress.GetFinishTime()
		if err := opuser.UpsertUserRefreshInfo(ctx, suid, info); err != nil {
			return nil, err
		}
		response = &papi.ActivityUseRefreshItemResponse{
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

func (s *ActivityServer) FinishRefresh(ctx context.Context, _ *papi.Empty) (*papi.ActivityFinishRefreshResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityFinishRefreshResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserRefreshRun(ctx, suid)
		if err != nil {
			return nil, activityRunError("refresh", err)
		}
		now := time.Now().UnixMilli()
		if now < progress.GetFinishTime() {
			return nil, status.Error(codes.FailedPrecondition, "refresh is not complete")
		}
		settingID, err := config.GetGameSettingId()
		if err != nil {
			return nil, err
		}
		setting, err := opmaster.FindSetting(ctx, settingID)
		if err != nil {
			return nil, err
		}
		overflowCoefficient := setting.GetActivityRefreshOverflowLimitCoefficientPermil()
		characters := make([]*ptransaction.UserCharacter, 0, len(progress.GetCharacters()))
		for _, selected := range progress.GetCharacters() {
			character, err := opuser.FindUserCharacter(ctx, suid, selected.GetCharacterId())
			if err != nil {
				return nil, err
			}
			level, err := opmaster.FindCharacterActivityLevel(ctx, int64(character.GetActivityLevel()))
			if err != nil {
				return nil, err
			}
			limit := int64(level.GetMaxStamina())
			if overflowCoefficient > 0 {
				limit = limit * int64(overflowCoefficient) / 1000
			}
			if limit < int64(level.GetMaxStamina()) {
				limit = int64(level.GetMaxStamina())
			}
			recovered := int64(character.GetActivityStamina()) + int64(progress.GetRecoveryAmount())
			if recovered > limit {
				recovered = limit
			}
			character.ActivityStamina = promotionInt32(recovered)
			character.StaminaUpdatedTime = now
			character.InActivity = false
			if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
				return nil, err
			}
			characters = append(characters, character)
		}
		info, err := opuser.FindUserRefreshInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		refresh := activityRefreshByID(info, progress.GetActivityRefreshId())
		if refresh == nil {
			return nil, status.Error(codes.FailedPrecondition, "active refresh definition is missing")
		}
		before := &papi.ActivityLevelInfo{
			Exp:   refresh.GetExp(),
			Level: refresh.GetLevel(),
		}
		after := &papi.ActivityLevelInfo{
			Exp:   refresh.GetExp(),
			Level: refresh.GetLevel(),
		}
		refresh.Exp = promotionInt32(int64(refresh.GetExp()) + 1)
		after.Exp = refresh.GetExp()
		totalCount, err := opuser.FindUserTotalCount(ctx, suid)
		if err != nil {
			return nil, err
		}
		if totalCount.GetActivityRefreshCount() == math.MaxInt64 {
			return nil, status.Error(codes.FailedPrecondition, "activity refresh count overflows")
		}
		totalCount.ActivityRefreshCount++
		if err := opuser.UpsertUserTotalCount(ctx, suid, totalCount); err != nil {
			return nil, err
		}
		info.ActiveRefresh = nil
		if err := opuser.UpsertUserRefreshInfo(ctx, suid, info); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserRefreshRun(ctx, suid); err != nil {
			return nil, err
		}
		maxLevel := int32(0)
		for _, level := range refresh.GetLevels() {
			if level.GetIsUnlocked() && level.GetLevel() > maxLevel {
				maxLevel = level.GetLevel()
			}
		}
		if maxLevel == 0 {
			maxLevel = refresh.GetLevel()
		}
		updated := &papi.UpdatedData{Characters: characters, TotalCount: totalCount}
		response = &papi.ActivityFinishRefreshResponse{
			ActivityLevelProgress: &papi.ActivityLevelProgress{
				MaxActivityLevel: maxLevel,
				Before:           before,
				After:            after,
			},
			Characters:     progress.GetCharacters(),
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func activityRefreshByID(info *papi.RefreshInfo, id string) *papi.Refresh {
	for _, refresh := range info.GetRefreshes() {
		if refresh.GetId() == id {
			return refresh
		}
	}
	return nil
}

func activityRefreshLevel(refresh *papi.Refresh, level int32) *papi.RefreshLevel {
	for _, candidate := range refresh.GetLevels() {
		if candidate.GetLevel() == level {
			return candidate
		}
	}
	return nil
}
