package opsolis

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindServerProfile(ctx context.Context) (*smodel.ServerProfile, error) {
	return scommon.FindOne[smodel.ServerProfile](
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

// UpdateServerProfileOfficialMasterVersion advances the active version and the
// clean official baseline together. Custom publications must use
// UpdateServerProfileMasterVersion instead, preserving this baseline.
func UpdateServerProfileOfficialMasterVersion(ctx context.Context, version string) error {
	return scommon.UpdateOne(
		ctx,
		sname.ServerProfile,
		bson.M{},
		&struct {
			MasterVersion     string `bson:"masterVersion"`
			MasterVersionOrig string `bson:"masterVersionOrig"`
		}{MasterVersion: version, MasterVersionOrig: version},
	)
}

func DeleteServerProfile(ctx context.Context) error {
	return scommon.Delete(
		ctx,
		sname.ServerProfile,
		bson.M{},
	)
}

func InsertServerProfile(ctx context.Context, serverProfile *smodel.ServerProfile) error {
	return scommon.InsertOne(
		ctx,
		sname.ServerProfile,
		serverProfile,
	)
}
