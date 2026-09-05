package guild

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

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
