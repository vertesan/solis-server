package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/ptransaction"

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
		options.Find().SetSort(bson.D{{Key: "ShootingTime", Value: -1}}),
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
		&ustruct.UserStruct[*ptransaction.UserPhoto]{
			SUID: suid,
			Data: photo,
		},
	)
}

// Note: This function is preferred to be used inside a transaction if there are more than 1 objects in the slice
func UpsertManyUserPhotos(ctx context.Context, suid string, photos []*ptransaction.UserPhoto) error {
	for _, photo := range photos {
		if err := UpsertUserPhoto(ctx, suid, photo); err != nil {
			return err
		}
	}
	return nil
}

func DeleteUserPhoto(ctx context.Context, suid string, photoId string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPhoto,
		bson.M{"SUID": suid, "photoId": photoId},
	)
}

func DeleteAllUserPhotos(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPhoto,
		bson.M{"SUID": suid},
	)
}
