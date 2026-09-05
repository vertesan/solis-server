package activity

import (
	"context"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
