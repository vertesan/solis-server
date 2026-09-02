package opmaster

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindGuild(ctx context.Context, id string) (*pmaster.Guild, error) {
	return mcommon.FindOne[mmodel.Guild, pmaster.Guild](
		ctx,
		mname.Guild,
		bson.M{"Id": id},
	)
}

func FindAllGuilds(ctx context.Context) ([]*pmaster.Guild, error) {
	guilds, err := mcommon.FindMany[mmodel.Guild, pmaster.Guild](
		ctx,
		mname.Guild,
		bson.M{},
	)
	if err != nil {
		return nil, err
	}
	sort.Slice(guilds, func(i, j int) bool {
		if guilds[i].GetOrder() != guilds[j].GetOrder() {
			return guilds[i].GetOrder() < guilds[j].GetOrder()
		}
		return guilds[i].GetId() < guilds[j].GetId()
	})
	return guilds, nil
}
