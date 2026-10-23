package test

import (
	"bytes"
	"context"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/proto"
)

// Exercise the documented export/edit/import workflow without dropping the
// normal shop's existing items, product mappings, or other shops.
func TestMakinoNormalShopEditScenario(t *testing.T) {
	binary := os.Getenv("MAKINO_TEST_CLI_BINARY")
	if binary == "" {
		t.Skip("set MAKINO_TEST_CLI_BINARY to test editing an existing shop")
	}
	backupMakinoCatalogCollections(t, sname.ShopCatalog)
	setupTestUser(t)
	ctx := context.Background()
	before, err := opsolis.FindAllShopCatalogEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	beforeByID := make(map[string]*smodel.ShopCatalogEntry, len(before))
	for _, shop := range before {
		beforeByID[shop.ShopId] = shop
	}
	if beforeByID["shop_top_shop"] == nil {
		t.Skip("current server catalog has no normal shop")
	}
	run := func(args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		command := exec.CommandContext(ctx, binary, args...)
		isolateTestCommand(t, command, "..")
		var stderr bytes.Buffer
		command.Stderr = &stderr
		data, err := command.Output()
		if err != nil {
			t.Fatalf("makino command failed: %v\n%s", err, stderr.String())
		}
		return data
	}
	var document map[string]any
	if err := yaml.Unmarshal(run("server", "shop", "--export"), &document); err != nil {
		t.Fatal(err)
	}
	const itemID = "shop_item_makino_edit-test-blue-dia-160"
	for _, value := range document["shops"].([]any) {
		shop := value.(map[string]any)
		if shop["shopId"] == "shop_top_shop" {
			shop["items"] = append(shop["items"].([]any), map[string]any{
				"id": itemID, "name": "ブルーダイヤ×160（追加）", "order": 1000,
				"thumbnailAssetId": "img_shop_item_thumb_blue-dia-120",
				"rewardId":         "rd_auto;paid_stone;;160", "consumptionId": "", "unlocked": true,
			})
		}
	}
	edited, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "shops.yaml")
	if err := os.WriteFile(path, edited, 0600); err != nil {
		t.Fatal(err)
	}
	run("server", "shop", "--overwrite", "--file", path)
	after, err := opsolis.FindAllShopCatalogEntries(ctx)
	if err != nil || len(after) != len(before) {
		t.Fatalf("editing one shop changed the shop roster: %v", err)
	}
	for _, shop := range after {
		old := beforeByID[shop.ShopId]
		if old == nil || !proto.Equal(old.Info, shop.Info) ||
			!proto.Equal(old.LoginBonusPackageItem, shop.LoginBonusPackageItem) ||
			!maps.Equal(old.OriginalProductIds, shop.OriginalProductIds) {
			t.Fatalf("editing one item changed shop metadata or diamond mappings for %q", shop.ShopId)
		}
		wantCount := len(old.Items)
		if shop.ShopId == "shop_top_shop" {
			wantCount++
		}
		if len(shop.Items) != wantCount {
			t.Fatalf("shop %q has %d items, want %d", shop.ShopId, len(shop.Items), wantCount)
		}
		for i, item := range old.Items {
			if !proto.Equal(item, shop.Items[i]) {
				t.Fatalf("editing the normal shop changed existing item %q", item.GetId())
			}
		}
	}
	client := papi.NewShopClient(testConfig.GrpcConn)
	listed := profileCall(t, client.ListItem, &papi.ShopListItemRequest{ShopId: "shop_top_shop"})
	found := false
	for _, item := range listed.GetShopItems() {
		found = found || item.GetId() == itemID
	}
	if !found {
		t.Fatal("the added item was not visible without restarting the server")
	}
	balanceBefore, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	profileCall(t, client.Purchase, &papi.ShopPurchaseRequest{ShopItemId: itemID})
	balanceAfter, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	if err != nil || balanceAfter.GetPaidBalance() != balanceBefore.GetPaidBalance()+160 || balanceAfter.GetFreeBalance() != balanceBefore.GetFreeBalance() {
		t.Fatalf("the documented free bundle did not grant exactly 160 blue diamonds: %v", err)
	}
}
