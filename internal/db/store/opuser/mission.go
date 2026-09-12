package opuser

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindAllUserMissions(ctx context.Context, suid string) ([]*ptransaction.UserMission, error) {
	return ucommon.FindAll[ptransaction.UserMission](
		ctx,
		uname.UserMission,
		bson.M{"SUID": suid},
	)
}

func FindUserMission(ctx context.Context, suid, missionID string) (*ptransaction.UserMission, error) {
	return ucommon.FindOne[ptransaction.UserMission](ctx, uname.UserMission, bson.M{"SUID": suid, "missionId": missionID})
}

func UpsertUserMission(ctx context.Context, suid string, mission *ptransaction.UserMission) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserMission,
		bson.M{"SUID": suid, "missionId": mission.MissionId},
		&umodel.UserStruct[*ptransaction.UserMission]{
			SUID: suid,
			Data: mission,
		},
	)
}

func UpsertManyUserMissions(ctx context.Context, suid string, missions []*ptransaction.UserMission) error {
	if len(missions) == 0 {
		return nil
	}
	models := make([]mongo.WriteModel, 0, len(missions))
	for _, mission := range missions {
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"SUID": suid, "missionId": mission.MissionId}).
			SetUpdate(bson.M{"$set": &umodel.UserStruct[*ptransaction.UserMission]{
				SUID: suid,
				Data: mission,
			}}).
			SetUpsert(true))
	}
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := depot.User.Collection(uname.UserMission).BulkWrite(
			ctx,
			models,
			options.BulkWrite().SetOrdered(false),
		); err != nil {
			return nil, fmt.Errorf("failed to upsert user missions: %w", err)
		}
		return nil, nil
	})
}
