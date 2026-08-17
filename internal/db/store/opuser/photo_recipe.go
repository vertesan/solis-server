package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserPhotoRecipe(ctx context.Context, suid, photoRecipeId string) (*ptransaction.UserPhotoRecipe, error) {
	return ucommon.FindOne[ptransaction.UserPhotoRecipe](
		ctx, uname.UserPhotoRecipe, bson.M{"SUID": suid, "photoRecipeId": photoRecipeId},
	)
}

func FindAllUserPhotoRecipes(ctx context.Context, suid string) ([]*ptransaction.UserPhotoRecipe, error) {
	return ucommon.FindAll[ptransaction.UserPhotoRecipe](
		ctx,
		uname.UserPhotoRecipe,
		bson.M{"SUID": suid},
	)
}

func UpsertUserPhotoRecipe(ctx context.Context, suid string, recipe *ptransaction.UserPhotoRecipe) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserPhotoRecipe,
		bson.M{"SUID": suid, "photoRecipeId": recipe.PhotoRecipeId},
		&umodel.UserStruct[*ptransaction.UserPhotoRecipe]{SUID: suid, Data: recipe},
	)
}
