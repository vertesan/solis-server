package opsolis

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/solis/sdoc"
	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindServerProfile(ctx context.Context) (*smodel.ServerProfile, error) {
	return scommon.FindOne[smodel.ServerProfile](
		ctx,
		sname.ServerProfile,
		bson.M{"_id": sdoc.ServerProfileDocumentID},
	)
}

func UpdateServerProfileMasterVersion(ctx context.Context, version string) error {
	return scommon.UpdateOne(
		ctx,
		sname.ServerProfile,
		bson.M{"_id": sdoc.ServerProfileDocumentID},
		&struct {
			MasterVersion string `bson:"masterVersion"`
		}{MasterVersion: version},
	)
}

func DeleteServerProfile(ctx context.Context) error {
	return scommon.Delete(
		ctx,
		sname.ServerProfile,
		bson.M{"_id": sdoc.ServerProfileDocumentID},
	)
}

func InsertServerProfile(ctx context.Context, serverProfile *smodel.ServerProfile) error {
	return scommon.InsertOne(
		ctx,
		sname.ServerProfile,
		&struct {
			ID                    string `bson:"_id"`
			*smodel.ServerProfile `bson:",inline"`
		}{ID: sdoc.ServerProfileDocumentID, ServerProfile: serverProfile},
	)
}
