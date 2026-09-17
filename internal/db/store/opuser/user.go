package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindUser(ctx context.Context, suid string) (*ptransaction.User, error) {
	return ucommon.FindOne[ptransaction.User](
		ctx,
		uname.User,
		bson.M{"SUID": suid},
	)
}

// LockUserQuestStart performs a real write to the existing user document inside
// the live transaction. Concurrent starts retry before expensive input/simulation
// work. This server-only revision is deliberately absent from User protobuf;
// ordinary UpsertUser uses $set and therefore preserves it.
func LockUserQuestStart(ctx context.Context, suid string) (*ptransaction.User, error) {
	return FindAndUpdateUser(ctx, suid, bson.M{"$inc": bson.M{"questStartRevision": int64(1)}},
		options.FindOneAndUpdate().SetReturnDocument(options.After))
}

func UpsertUser(ctx context.Context, suid string, user *ptransaction.User) error {
	return ucommon.UpsertOne(
		ctx,
		uname.User,
		bson.M{"SUID": suid},
		&umodel.UserStruct[*ptransaction.User]{
			SUID: suid,
			Data: user,
		},
	)
}

func FindAndUpdateUser(ctx context.Context, suid string, update map[string]any, opts ...options.Lister[options.FindOneAndUpdateOptions]) (*ptransaction.User, error) {
	return ucommon.FindAndUpdateOne[ptransaction.User](
		ctx,
		uname.User,
		bson.M{"SUID": suid},
		update,
		opts...,
	)
}

func DeleteUser(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.User,
		bson.M{"SUID": suid},
	)
}
