package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserDecorations(ctx context.Context, suid string) ([]*ptransaction.UserDecoration, error) {
	return ucommon.FindAll[ptransaction.UserDecoration](
		ctx,
		uname.UserDecoration,
		bson.M{"SUID": suid},
	)
}

func FindUserDecoration(ctx context.Context, suid, decorationID string) (*ptransaction.UserDecoration, error) {
	return ucommon.FindOne[ptransaction.UserDecoration](
		ctx,
		uname.UserDecoration,
		bson.M{"SUID": suid, "decorationId": decorationID},
	)
}

func UpsertUserDecoration(ctx context.Context, suid string, decoration *ptransaction.UserDecoration) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserDecoration,
		bson.M{"SUID": suid, "decorationId": decoration.GetDecorationId()},
		&umodel.UserStruct[*ptransaction.UserDecoration]{SUID: suid, Data: decoration},
	)
}
