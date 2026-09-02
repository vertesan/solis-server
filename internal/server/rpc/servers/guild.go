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
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const guildDailyCheckInMedalAmount int32 = 100

type GuildServer struct {
	papi.UnimplementedGuildServer
}

func (s *GuildServer) List(ctx context.Context, _ *papi.Empty) (*papi.GuildListResponse, error) {
	masterGuilds, err := opmaster.FindAllGuilds(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}

	guilds := make([]*papi.GuildParam, 0, len(masterGuilds))
	for index, guild := range masterGuilds {
		rank := int32(index + 1)
		guilds = append(guilds, &papi.GuildParam{
			Id:               guild.GetId(),
			WeaknessRank:     rank,
			MemberAmountRank: rank,
			PrevSeasonResult: &papi.GuildGvgSeasonResult{},
		})
	}

	return &papi.GuildListResponse{
		Guilds:         guilds,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

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
			rewardAmount = guildDailyCheckInMedalAmount

			user.LastGuildCheckTime = now
			if err := opuser.UpsertUser(ctx, suid, user); err != nil {
				return nil, err
			}
			updated.User = user

			if _, err := viber.GrantExchangeRewards(ctx, suid, &pmaster.RewardResource{
				ResourceType: penum.ResourceType_ResourceType_GuildMedal,
				Amount:       guildDailyCheckInMedalAmount,
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

func (s *GuildServer) Join(ctx context.Context, req *papi.GuildJoinRequest) (*papi.GuildJoinResponse, error) {
	if req == nil || req.GetGuildId() == "" {
		return nil, viber.InvalidArgumentf("guildId is required")
	}
	if _, err := opmaster.FindGuild(ctx, req.GetGuildId()); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, viber.InvalidArgumentf("unknown guildId %q", req.GetGuildId())
		}
		return nil, viber.GameplayError(err)
	}

	suid := viber.GetSuid(ctx).String()
	now := time.Now().UnixMilli()
	var response *papi.GuildJoinResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		user, err := opuser.FindAndUpdateUser(
			ctx,
			suid,
			bson.M{"$set": bson.M{
				"guildId":            req.GetGuildId(),
				"lastGuildCheckTime": now,
			}},
			options.FindOneAndUpdate().SetReturnDocument(options.After),
		)
		if err != nil {
			return nil, err
		}
		response = &papi.GuildJoinResponse{
			CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{User: user}),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
