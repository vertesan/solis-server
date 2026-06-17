package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserPhotoRecipes(ctx context.Context, suid string) ([]*ptransaction.UserPhotoRecipe, error) {
	return ucommon.FindAll[ptransaction.UserPhotoRecipe](
		ctx,
		uname.UserPhotoRecipe,
		bson.M{"SUID": suid},
	)
}
