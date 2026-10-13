package test

import (
	"context"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPermanentGachaRosterScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	masters, err := opmaster.FindAllGachas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]*pmaster.Gacha, len(masters))
	for _, master := range masters {
		byID[master.GetId()] = master
	}
	client := papi.NewGachaClient(testConfig.GrpcConn)
	userClient := papi.NewUserClient(testConfig.GrpcConn)
	homeClient := papi.NewHomeClient(testConfig.GrpcConn)
	checkRoster := func() []*papi.GachaInfo {
		t.Helper()
		grpcCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		user, err := userClient.Get(grpcCtx, &papi.Empty{})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		grpcCtx, cancel, err = getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		home, err := homeClient.Enter(grpcCtx, &papi.HomeEnterRequest{})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		grpcCtx, cancel, err = getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		list, err := client.List(grpcCtx, &papi.Empty{})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		assertStringRoster(t, "Home/Gacha.List filtered roster", testGachaInfoIDs(home.GetGachaInfos()), testGachaInfoIDs(list.GetGachas()))
		assertStringRoster(t, "User.Get filtered buttons", testUserGachaButtonIDs(user.GetGachaButtons()), testGachaInfoButtonIDs(list.GetGachas()))
		var normal, nonCard int
		for _, info := range list.GetGachas() {
			master := byID[info.GetId()]
			if master == nil {
				t.Fatalf("unknown gacha %q", info.GetId())
			}
			switch master.GetGachaType() {
			case penum.GachaType_GachaType_Item, penum.GachaType_GachaType_Premium, penum.GachaType_GachaType_ShowcaseToy:
				nonCard++
			default:
				if master.GetIsPeriodDisplay() || strings.Contains(master.GetId(), "fixed") || strings.Contains(master.GetName(), "誕生日") || strings.Contains(master.GetName(), "初心者") || strings.Contains(master.GetId(), "beginner") || strings.Contains(master.GetId(), "free-ticket") {
					t.Fatalf("excluded card gacha returned to the client: %s (%s)", master.GetId(), master.GetName())
				}
				if strings.HasPrefix(master.GetId(), "gacha-normal-") {
					normal++
				}
			}
		}
		if normal == 0 || nonCard == 0 {
			t.Fatalf("filter removed ordinary/non-card gachas: normal=%d nonCard=%d", normal, nonCard)
		}
		return list.GetGachas()
	}
	visible := checkRoster()
	stored, err := opuser.FindAllUserGachaInfos(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	assertStringRoster(t, "new user initialized only visible gachas", testGachaInfoIDs(stored), testGachaInfoIDs(visible))

	// Existing accounts may still have rows from an older roster. They must
	// neither leak into lists nor become callable through a stale client page.
	legacyIDs := []string{
		"gacha-beginner-21-1126",
		"gacha-beginner-fixed-selection-24-1228",
		"gacha-free-ticket-1s-26-0910",
		"gacha-birthday-26-0913",
	}
	for _, id := range legacyIDs {
		master := byID[id]
		if master == nil {
			t.Fatalf("missing legacy gacha fixture %q", id)
		}
		legacy := &papi.GachaInfo{Id: id, Unlocked: true}
		for _, buttonID := range master.GetButtonIds() {
			legacy.Buttons = append(legacy.Buttons, &papi.GachaButtonInfo{Id: buttonID})
		}
		if err := opuser.UpsertUserGachaInfo(ctx, testConfig.TestUserSuid, legacy); err != nil {
			t.Fatal(err)
		}
		grpcCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Probability(grpcCtx, &papi.GachaProbabilityRequest{GachaId: id})
		cancel()
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("disabled gacha %q remained callable: %v", id, err)
		}
	}
	promoteTestUser(t)
	// Guaranteed-rarity tickets are permanent and must remain usable; they
	// are not the fixed-pool promotional gachas being excluded.
	if err := opuser.UpsertUserItem(ctx, testConfig.TestUserSuid, &ptransaction.UserItem{ItemId: "item-gacha_ticket-r5", Amount: 1}); err != nil {
		t.Fatal(err)
	}
	visible = checkRoster()
	for _, info := range visible {
		if info.GetId() == "gacha-ticket-21-0816-01-r5" {
			return
		}
	}
	t.Fatal("owning a permanent guaranteed-rarity ticket did not expose its gacha")
}
