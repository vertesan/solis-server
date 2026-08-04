package opsolis

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindAllDivisions(ctx context.Context) ([]*papi.DivisionInfo, error) {
	return scommon.FindAll[papi.DivisionInfo](
		ctx,
		sname.Division,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "levelAvg", Value: 1}}),
	)
}

func UpsertDivision(ctx context.Context, divisionInfo *papi.DivisionInfo) error {
	return scommon.UpsertOne(
		ctx,
		sname.Division,
		bson.M{"id": divisionInfo.Id},
		divisionInfo,
	)
}

func DeleteAllDivisions(ctx context.Context) error {
	return scommon.Delete(
		ctx,
		sname.Division,
		bson.M{},
	)
}
