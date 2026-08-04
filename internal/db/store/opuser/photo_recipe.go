package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserPhotoRecipes(ctx context.Context, suid string) ([]*ptransaction.UserPhotoRecipe, error) {
	return ucommon.FindAll[ptransaction.UserPhotoRecipe](
		ctx,
		uname.UserPhotoRecipe,
		bson.M{"SUID": suid},
	)
}
