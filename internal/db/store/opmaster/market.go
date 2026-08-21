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

func FindMarketAccessoriesByRarity(ctx context.Context, rarity int32) ([]*pmaster.Accessory, error) {
	accessories, err := mcommon.FindMany[mmodel.Accessory, pmaster.Accessory](ctx, mname.Accessory, bson.M{})
	if err != nil {
		return nil, err
	}
	filtered := make([]*pmaster.Accessory, 0)
	for _, accessory := range accessories {
		if accessory.GetRarity() == rarity {
			filtered = append(filtered, accessory)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].GetId() < filtered[j].GetId() })
	return filtered, nil
}
