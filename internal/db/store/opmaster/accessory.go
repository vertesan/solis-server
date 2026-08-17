package opmaster

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func FindAccessory(ctx context.Context, id string) (*pmaster.Accessory, error) {
	return mcommon.FindOne[mmodel.Accessory, pmaster.Accessory](
		ctx, mname.Accessory, bson.M{"Id": id},
	)
}

func FindAccessoryByClassificationAndPhase(ctx context.Context, accessory *pmaster.Accessory, phase int32) (*pmaster.Accessory, error) {
	lineage, err := accessoryLineage(accessory.Id)
	if err != nil {
		return nil, err
	}
	accessories, err := findAllAccessories(ctx)
	if err != nil {
		return nil, err
	}
	var match *pmaster.Accessory
	for _, candidate := range accessories {
		candidateLineage, err := accessoryLineage(candidate.Id)
		if err != nil {
			continue
		}
		if candidateLineage == lineage &&
			candidate.Classification == accessory.Classification &&
			candidate.AssetId == accessory.AssetId &&
			candidate.LimitBreakPhase == phase &&
			candidate.CategoryType == accessory.CategoryType &&
			candidate.CharacterId == accessory.CharacterId &&
			candidate.CharacterGroupId == accessory.CharacterGroupId {
			if match != nil {
				return nil, fmt.Errorf("multiple previous accessory phases found for %q", accessory.Id)
			}
			match = candidate
		}
	}
	if match != nil {
		return match, nil
	}
	return nil, fmt.Errorf("previous accessory phase: %w", mongo.ErrNoDocuments)
}

func FindAccessoryEnhancementMaterial(ctx context.Context, target *pmaster.Accessory) (*pmaster.Accessory, error) {
	lineage, sequence, width, err := accessorySequence(target.Id)
	if err != nil {
		return nil, err
	}
	if sequence == 0 {
		return nil, fmt.Errorf("accessory %q has no previous enhancement sequence", target.Id)
	}
	materialId := fmt.Sprintf("%s%0*d", lineage, width, sequence-1)
	material, err := FindAccessory(ctx, materialId)
	if err != nil {
		return nil, err
	}
	if material.Classification != target.Classification ||
		material.Rarity != target.Rarity-1 ||
		material.LimitBreakPhase != target.LimitBreakPhase ||
		material.CategoryType != target.CategoryType ||
		material.CharacterId != target.CharacterId ||
		material.CharacterGroupId != target.CharacterGroupId {
		return nil, fmt.Errorf("accessory %q is not the valid enhancement predecessor of %q", material.Id, target.Id)
	}
	return material, nil
}

func FindAccessoryEnhancement(ctx context.Context, rarity int32) (*pmaster.AccessoryEnhancement, error) {
	return mcommon.FindOne[mmodel.AccessoryEnhancement, pmaster.AccessoryEnhancement](
		ctx, mname.AccessoryEnhancement, bson.M{"Rarity": rarity},
	)
}

func findAllAccessories(ctx context.Context) ([]*pmaster.Accessory, error) {
	return mcommon.FindMany[mmodel.Accessory, pmaster.Accessory](ctx, mname.Accessory, bson.M{})
}

func accessoryLineage(id string) (string, error) {
	lineage, _, _, err := accessorySequence(id)
	return lineage, err
}

func accessorySequence(id string) (string, uint64, int, error) {
	separator := strings.LastIndexByte(id, '-')
	if separator < 0 || separator == len(id)-1 {
		return "", 0, 0, fmt.Errorf("accessory id %q has no numeric phase suffix", id)
	}
	suffix := id[separator+1:]
	sequence, err := strconv.ParseUint(suffix, 10, 32)
	if err != nil {
		return "", 0, 0, fmt.Errorf("accessory id %q has an invalid phase suffix: %w", id, err)
	}
	return id[:separator+1], sequence, len(suffix), nil
}
