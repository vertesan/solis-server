package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindUserPhoto(ctx context.Context, suid string, photoId string) (*ptransaction.UserPhoto, error) {
	return ucommon.FindOne[ptransaction.UserPhoto](
		ctx,
		uname.UserPhoto,
		bson.M{"SUID": suid, "photoId": photoId},
	)
}

func FindAllUserPhotos(ctx context.Context, suid string) ([]*ptransaction.UserPhoto, error) {
	return ucommon.FindAll[ptransaction.UserPhoto](
		ctx,
		uname.UserPhoto,
		bson.M{"SUID": suid},
		options.Find().SetSort(bson.D{{Key: "shootingTime", Value: -1}}),
	)
}

func UpsertUserPhoto(ctx context.Context, suid string, photo *ptransaction.UserPhoto) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserPhoto,
		bson.M{
			"SUID":    suid,
			"photoId": photo.PhotoId,
		},
		&umodel.UserStruct[*ptransaction.UserPhoto]{
			SUID: suid,
			Data: photo,
		},
	)
}

func UpsertManyUserPhotos(ctx context.Context, suid string, photos []*ptransaction.UserPhoto) error {
	return ucommon.ForEachAtomically(ctx, photos, func(ctx context.Context, photo *ptransaction.UserPhoto) error {
		return UpsertUserPhoto(ctx, suid, photo)
	})
}

func DeleteUserPhoto(ctx context.Context, suid string, photoId string) error {
	return DeleteUserPhotos(ctx, suid, []string{photoId})
}

func DeleteUserPhotos(ctx context.Context, suid string, photoIds []string) error {
	if len(photoIds) == 0 {
		return nil
	}
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := ucommon.Delete(ctx, uname.UserPhoto, bson.M{"SUID": suid, "photoId": bson.M{"$in": photoIds}}); err != nil {
			return nil, err
		}
		return nil, ucommon.Delete(ctx, uname.UserDreamArea, bson.M{"SUID": suid, "basePhotoId": bson.M{"$in": photoIds}})
	})
}

func DeleteAllUserPhotos(ctx context.Context, suid string) error {
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := ucommon.Delete(ctx, uname.UserPhoto, bson.M{"SUID": suid}); err != nil {
			return nil, err
		}
		return nil, ucommon.Delete(ctx, uname.UserDreamArea, bson.M{"SUID": suid})
	})
}

// FindUserPhotoGift returns a fully generated photo that is waiting in the
// gift box because the user's owned-photo capacity was full.
func FindUserPhotoGift(ctx context.Context, suid, photoId string) (*ptransaction.UserPhoto, error) {
	return ucommon.FindOne[ptransaction.UserPhoto](
		ctx,
		uname.UserPhotoGift,
		bson.M{"SUID": suid, "photoId": photoId},
	)
}

func UpsertUserPhotoGift(ctx context.Context, suid string, photo *ptransaction.UserPhoto) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserPhotoGift,
		bson.M{"SUID": suid, "photoId": photo.GetPhotoId()},
		&umodel.UserStruct[*ptransaction.UserPhoto]{SUID: suid, Data: photo},
	)
}

func DeleteUserPhotoGift(ctx context.Context, suid, photoId string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPhotoGift,
		bson.M{"SUID": suid, "photoId": photoId},
	)
}
