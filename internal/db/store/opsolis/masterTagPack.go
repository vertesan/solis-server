package opsolis

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func UpsertMasterTagPack(ctx context.Context, masterTagPack *ptransaction.MasterTagPack) error {
	return scommon.UpsertOne(
		ctx,
		sname.MasterTagPack,
		bson.M{"type": masterTagPack.Type},
		masterTagPack,
	)
}

func GetAllMasterTagPack(ctx context.Context) ([](*ptransaction.MasterTagPack), error) {
	return scommon.FindAll[ptransaction.MasterTagPack](
		ctx,
		sname.MasterTagPack,
		bson.M{},
	)
}

func GetAllOriginalMasterTagPack(ctx context.Context) ([]*ptransaction.MasterTagPack, error) {
	return scommon.FindAll[ptransaction.MasterTagPack](ctx, sname.MasterTagPackOrig, bson.M{})
}

// ReplaceOfficialMasterTag stores a clean official manifest, never a copy of
// the potentially customized active manifest. Its transaction is shared with
// the caller when the master collections are being replaced at startup.
func ReplaceOfficialMasterTag(ctx context.Context, tag *ptransaction.MasterTag) error {
	if tag == nil || tag.Version == "" || len(tag.MasterTagPacks) == 0 {
		return fmt.Errorf("official master tag must contain a version and packs")
	}
	types := make(map[string]struct{}, len(tag.MasterTagPacks))
	for _, pack := range tag.MasterTagPacks {
		if pack == nil || pack.Type == "" {
			return fmt.Errorf("official master tag contains an invalid pack")
		}
		if _, duplicate := types[pack.Type]; duplicate {
			return fmt.Errorf("duplicate official master type %q", pack.Type)
		}
		types[pack.Type] = struct{}{}
	}
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		// Replacement also removes types retired by the official release.
		for _, collection := range []string{sname.MasterTagPack, sname.MasterTagPackOrig} {
			if err := scommon.Delete(ctx, collection, bson.M{}); err != nil {
				return nil, err
			}
			for _, pack := range tag.MasterTagPacks {
				if err := scommon.UpsertOne(ctx, collection, bson.M{"type": pack.Type}, pack); err != nil {
					return nil, err
				}
			}
		}
		return nil, UpdateServerProfileOfficialMasterVersion(ctx, tag.Version)
	})
}
