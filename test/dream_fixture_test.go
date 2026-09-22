package test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/protobuf/proto"
)

func dreamTestPhotos(t *testing.T, ctx context.Context, suid string) (*ptransaction.UserPhoto, *ptransaction.UserPhoto) {
	t.Helper()
	if suid != testConfig.TestUserSuid {
		t.Fatal("Dream fixture must use the configured test user")
	}
	areas, _ := questCatalogFixture(t)
	var requiredOrder int32
	seen := map[string]bool{}
	var inspect func(string)
	inspect = func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		condition, err := opmaster.FindCondition(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, setting := range condition.GetSettings() {
			for _, area := range areas {
				if area.GetId() == setting.GetClearArea().GetAreaId() {
					requiredOrder = max(requiredOrder, area.GetOrder())
				}
			}
			inspect(setting.GetSatisfyCondition().GetConditionId())
		}
	}
	inspect(dream.UnlockConditionID)
	for _, area := range questAreasOfType(areas, penum.AreaType_AreaType_Main) {
		if area.GetOrder() <= requiredOrder {
			questSeedFullArea(t, area)
		}
	}
	maximum, err := opmaster.FindMaxManagerLevel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	questSetManagerLevel(t, maximum.GetLevel())
	ids := []string{"pab-vocal_multiply-4", "pab-dance_multiply-4", "pab-visual_multiply-4"}
	rows, err := opmaster.FindPhotoAbilities(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	masters := map[string]*pmaster.PhotoAbility{}
	for _, row := range rows {
		masters[row.GetId()] = row
	}
	base := &ptransaction.UserPhoto{PhotoId: uuid.NewString(), AssetId: uuid.NewString(), ImageType: penum.PhotoImageType_PhotoImageType_User,
		Name: "Dream base", Rarity: 4, Level: 30, Rerollable: true, Abilities: buildTestPhotoAbilities(t, &pmaster.PhotoAbilitySet{PhotoAbilityIds: ids}, masters, 30)}
	material := proto.Clone(base).(*ptransaction.UserPhoto)
	material.PhotoId = uuid.NewString()
	material.AssetId = uuid.NewString()
	material.Name = "Dream material"
	for _, photo := range []*ptransaction.UserPhoto{base, material} {
		if err := opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
			t.Fatal(err)
		}
	}
	return base, material
}
