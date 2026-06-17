package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserPhotoMusic(ctx context.Context, suid string, musicId string) (*papi.PhotoMusic, error) {
	return ucommon.FindOne[papi.PhotoMusic](
		ctx,
		uname.UserPhotoMusic,
		bson.M{"SUID": suid, "musicId": musicId},
	)
}

func FindAllUserPhotoMusics(ctx context.Context, suid string) ([]*papi.PhotoMusic, error) {
	return ucommon.FindAll[papi.PhotoMusic](
		ctx,
		uname.UserPhotoMusic,
		bson.M{"SUID": suid},
	)
}

func UpsertUserPhotoMusic(ctx context.Context, suid string, music *papi.PhotoMusic) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserPhotoMusic,
		bson.M{"SUID": suid, "musicId": music.MusicId},
		&ustruct.UserStruct[*papi.PhotoMusic]{
			SUID: suid,
			Data: music,
		},
	)
}

func UpsertManyUserPhotoMusics(ctx context.Context, suid string, musics []*papi.PhotoMusic) error {
	for _, m := range musics {
		if err := UpsertUserPhotoMusic(ctx, suid, m); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserPhotoMusics(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPhotoMusic,
		bson.M{"SUID": suid},
	)
}
