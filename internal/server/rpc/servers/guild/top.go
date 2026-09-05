package guild

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
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (s *GuildServer) Top(ctx context.Context, _ *papi.Empty) (*papi.GuildTopResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.GuildTopResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}

		var updated *papi.UpdatedData
		var rewardAmount int32
		now := time.Now().UnixMilli()
		if user.GetGuildId() != "" && !viber.SameJSTDay(user.GetLastGuildCheckTime(), now) {
			updated = &papi.UpdatedData{}
			rewardAmount, err = guildDailyCheckInReward(ctx, suid)
			if err != nil {
				return nil, err
			}

			user.LastGuildCheckTime = now
			if err := opuser.UpsertUser(ctx, suid, user); err != nil {
				return nil, err
			}
			updated.User = user

			if _, err := viber.GrantExchangeRewards(ctx, suid, &pmaster.RewardResource{
				ResourceType: penum.ResourceType_ResourceType_GuildMedal,
				Amount:       rewardAmount,
			}, updated); err != nil {
				return nil, err
			}

			totalCount, err := opuser.FindUserTotalCount(ctx, suid)
			if err != nil {
				if !errors.Is(err, mongo.ErrNoDocuments) {
					return nil, err
				}
				totalCount = &ptransaction.UserTotalCount{}
			}
			if totalCount.GetGuildCheckInCount() == math.MaxInt64 {
				return nil, viber.FailedPreconditionf("guild check-in count limit reached")
			}
			totalCount.GuildCheckInCount++
			if err := opuser.UpsertUserTotalCount(ctx, suid, totalCount); err != nil {
				return nil, err
			}
			updated.TotalCount = totalCount

			checkInMissions, err := viber.AdvanceUserMissions(
				ctx,
				suid,
				penum.MissionType_MissionType_GuildCheckInCount,
				1,
			)
			if err != nil {
				return nil, err
			}
			totalMissions, err := viber.AdvanceUserMissions(
				ctx,
				suid,
				penum.MissionType_MissionType_GuildCheckInTotalCount,
				1,
			)
			if err != nil {
				return nil, err
			}
			updated.Missions = appendUniqueGuildMissions(checkInMissions, totalMissions)
		}

		response = &papi.GuildTopResponse{
			RewardGuildMedalAmount: rewardAmount,
			GvgSeasonInfo:          nil,
			ExchangeBoothId:        "",
			CommonResponse:         viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

const (
	guildDailyCheckInBaseMedalAmount     int64 = 50
	guildDailyCheckInFallbackMedalAmount int32 = 100
)

func guildDailyCheckInReward(ctx context.Context, suid string) (int32, error) {
	staffs, err := opuser.FindAllUserStaffs(ctx, suid)
	if err != nil {
		return 0, err
	}
	enhanceCount, err := viber.CurrentStaffEnhanceCount(staffs)
	if err != nil {
		return 0, err
	}
	advantage, err := opmaster.FindTotalStaffEnhanceAdvantage(ctx, enhanceCount)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return guildDailyCheckInFallbackMedalAmount, nil
		}
		return 0, err
	}
	if !advantage.GetIsDataExist() {
		return guildDailyCheckInFallbackMedalAmount, nil
	}
	if advantage.GetAdvantagePermil() < 0 {
		return 0, viber.FailedPreconditionf("staff check-in advantage is invalid")
	}
	amount := guildDailyCheckInBaseMedalAmount +
		guildDailyCheckInBaseMedalAmount*int64(advantage.GetAdvantagePermil())/1000
	if amount <= 0 || amount > math.MaxInt32 {
		return 0, viber.FailedPreconditionf("guild check-in reward is invalid")
	}
	return int32(amount), nil
}

func appendUniqueGuildMissions(groups ...[]*ptransaction.UserMission) []*ptransaction.UserMission {
	result := make([]*ptransaction.UserMission, 0)
	indexByID := make(map[string]int)
	for _, group := range groups {
		for _, mission := range group {
			if index, exists := indexByID[mission.GetMissionId()]; exists {
				result[index] = mission
				continue
			}
			indexByID[mission.GetMissionId()] = len(result)
			result = append(result, mission)
		}
	}
	return result
}
