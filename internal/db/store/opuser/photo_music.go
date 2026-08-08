package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

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
		&umodel.UserStruct[*papi.PhotoMusic]{
			SUID: suid,
			Data: music,
		},
	)
}

func UpsertManyUserPhotoMusics(ctx context.Context, suid string, musics []*papi.PhotoMusic) error {
	return ucommon.ForEachAtomically(ctx, musics, func(ctx context.Context, music *papi.PhotoMusic) error {
		return UpsertUserPhotoMusic(ctx, suid, music)
	})
}

func DeleteAllUserPhotoMusics(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPhotoMusic,
		bson.M{"SUID": suid},
	)
}
