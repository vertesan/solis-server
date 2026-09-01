package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserShowcaseLayout(ctx context.Context, suid string, number int32) (*papi.ShowcaseLayoutInfo, error) {
	doc, err := ucommon.FindOne[umodel.UserShowcaseLayout](
		ctx,
		uname.UserShowcaseLayout,
		bson.M{"SUID": suid, "number": number},
	)
	if err != nil {
		return nil, err
	}
	return doc.LayoutInfo, nil
}

func UpsertUserShowcaseLayout(ctx context.Context, suid string, number int32, layout *papi.ShowcaseLayoutInfo) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserShowcaseLayout,
		bson.M{"SUID": suid, "number": number},
		&umodel.UserShowcaseLayout{SUID: suid, Number: number, LayoutInfo: layout},
	)
}

func DeleteUserShowcaseLayout(ctx context.Context, suid string, number int32) error {
	return ucommon.Delete(ctx, uname.UserShowcaseLayout, bson.M{"SUID": suid, "number": number})
}

func DeleteAllUserShowcaseLayouts(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserShowcaseLayout, bson.M{"SUID": suid})
}
