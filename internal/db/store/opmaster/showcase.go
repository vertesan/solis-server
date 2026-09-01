package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const DefaultShowcaseSettingID = "showcase-setting-default"

func FindShowcaseFrame(ctx context.Context, id string) (*pmaster.ShowcaseFrame, error) {
	return mcommon.FindOne[mmodel.ShowcaseFrame, pmaster.ShowcaseFrame](
		ctx, mname.ShowcaseFrame, bson.M{"Id": id},
	)
}

func FindAllShowcaseFrames(ctx context.Context) ([]*pmaster.ShowcaseFrame, error) {
	return mcommon.FindMany[mmodel.ShowcaseFrame, pmaster.ShowcaseFrame](
		ctx,
		mname.ShowcaseFrame,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "Id", Value: 1}}),
	)
}

func FindShowcaseMusic(ctx context.Context, musicID string) (*pmaster.ShowcaseMusic, error) {
	return mcommon.FindOne[mmodel.ShowcaseMusic, pmaster.ShowcaseMusic](
		ctx, mname.ShowcaseMusic, bson.M{"MusicId": musicID},
	)
}

func FindShowcaseSetting(ctx context.Context, id string) (*pmaster.ShowcaseSetting, error) {
	return mcommon.FindOne[mmodel.ShowcaseSetting, pmaster.ShowcaseSetting](
		ctx, mname.ShowcaseSetting, bson.M{"Id": id},
	)
}

func FindDefaultShowcaseSetting(ctx context.Context) (*pmaster.ShowcaseSetting, error) {
	return FindShowcaseSetting(ctx, DefaultShowcaseSettingID)
}

func FindAllShowcaseSettings(ctx context.Context) ([]*pmaster.ShowcaseSetting, error) {
	return mcommon.FindMany[mmodel.ShowcaseSetting, pmaster.ShowcaseSetting](
		ctx,
		mname.ShowcaseSetting,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "Id", Value: 1}}),
	)
}

func FindAllShowcaseHashtags(ctx context.Context) ([]*pmaster.ShowcaseHashtag, error) {
	return mcommon.FindMany[mmodel.ShowcaseHashtag, pmaster.ShowcaseHashtag](
		ctx,
		mname.ShowcaseHashtag,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "Id", Value: 1}}),
	)
}
