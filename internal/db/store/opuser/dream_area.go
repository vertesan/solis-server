package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserDreamArea(ctx context.Context, suid, photoID string) (*umodel.UserDreamArea, error) {
	return ucommon.FindOne[umodel.UserDreamArea](ctx, uname.UserDreamArea, bson.M{"_id": suid + ":" + photoID, "SUID": suid})
}

func FindAllUserDreamAreas(ctx context.Context, suid string) ([]*umodel.UserDreamArea, error) {
	return ucommon.FindAll[umodel.UserDreamArea](ctx, uname.UserDreamArea, bson.M{"SUID": suid})
}

func UpsertUserDreamArea(ctx context.Context, area *umodel.UserDreamArea) error {
	return ucommon.UpsertOne(ctx, uname.UserDreamArea, bson.M{"_id": area.SUID + ":" + area.BasePhotoID, "SUID": area.SUID}, area)
}

func DeleteUserDreamArea(ctx context.Context, suid, photoID string) error {
	return ucommon.Delete(ctx, uname.UserDreamArea, bson.M{"_id": suid + ":" + photoID, "SUID": suid})
}
