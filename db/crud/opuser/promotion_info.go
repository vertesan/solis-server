package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserPromotionInfo(ctx context.Context, suid string) (*papi.PromotionInfo, error) {
	return ucommon.FindOne[papi.PromotionInfo](
		ctx,
		uname.UserPromotionInfo,
		bson.M{"SUID": suid},
	)
}

func UpsertUserPromotionInfo(ctx context.Context, suid string, info *papi.PromotionInfo) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserPromotionInfo,
		bson.M{"SUID": suid},
		&ustruct.UserStruct[*papi.PromotionInfo]{
			SUID: suid,
			Data: info,
		},
	)
}

func DeleteUserPromotionInfo(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPromotionInfo,
		bson.M{"SUID": suid},
	)
}
