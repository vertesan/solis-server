package opsolis

import (
	"context"
	"vertesan/solis-server/db/def/solis/sname"
	"vertesan/solis-server/db/def/solis/sstruct"
	"vertesan/solis-server/db/store/opsolis/internal/scommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindServerProfile(ctx context.Context) (*sstruct.ServerProfile, error) {
	return scommon.FindOne[sstruct.ServerProfile](
		ctx,
		sname.ServerProfile,
		bson.M{},
	)
}

func UpdateServerProfileMasterVersion(ctx context.Context, version string) error {
	return scommon.UpdateOne(
		ctx,
		sname.ServerProfile,
		bson.M{},
		&struct {
			MasterVersion string `bson:"masterVersion"`
		}{MasterVersion: version},
	)
}

func DeleteServerProfile(ctx context.Context) error {
	return scommon.Delete(
		ctx,
		sname.ServerProfile,
		bson.M{},
	)
}

func InsertServerProfile(ctx context.Context, serverProfile *sstruct.ServerProfile) error {
	return scommon.InsertOne(
		ctx,
		sname.ServerProfile,
		serverProfile,
	)
}
