package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserShowcaseMysetLayout(ctx context.Context, suid string, number int32) (*papi.ShowcaseLayoutInfo, error) {
	doc, err := ucommon.FindOne[umodel.UserShowcaseMysetLayout](
		ctx,
		uname.UserShowcaseMysetLayout,
		bson.M{"SUID": suid, "number": number},
	)
	if err != nil {
		return nil, err
	}
	return doc.LayoutInfo, nil
}

func UpsertUserShowcaseMysetLayout(ctx context.Context, suid string, number int32, layout *papi.ShowcaseLayoutInfo) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserShowcaseMysetLayout,
		bson.M{"SUID": suid, "number": number},
		&umodel.UserShowcaseMysetLayout{SUID: suid, Number: number, LayoutInfo: layout},
	)
}

func DeleteUserShowcaseMysetLayout(ctx context.Context, suid string, number int32) error {
	return ucommon.Delete(ctx, uname.UserShowcaseMysetLayout, bson.M{"SUID": suid, "number": number})
}

func DeleteAllUserShowcaseMysetLayouts(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserShowcaseMysetLayout, bson.M{"SUID": suid})
}
