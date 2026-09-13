package test

import (
	"context"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"google.golang.org/protobuf/proto"
)

// Skipping unchanged Home writes must still persist the first daily gacha reset
// and retain selections, stamp progress and lifetime draw counters.
func TestHomeGachaDailyResetScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	client := papi.NewHomeClient(testConfig.GrpcConn)
	initial := profileCall(t, client.Enter, &papi.HomeEnterRequest{})
	info, master := chooseSelectableGacha(t, initial.GetGachaInfos())
	if info == nil {
		for _, candidate := range initial.GetGachaInfos() {
			if len(candidate.GetButtons()) > 0 {
				info = candidate
				break
			}
		}
	}
	if info == nil || len(info.GetButtons()) == 0 {
		t.Fatal("current Home gacha catalog has no button for daily reset")
	}
	seed := proto.Clone(info).(*papi.GachaInfo)
	if master != nil {
		seed.SelectedCardIds = append([]string(nil), master.GetSelectCardIds()[:master.GetSelectPickupAmount()]...)
	}
	seed.StampInfo = &papi.GachaStampInfo{CurrentSheetNumber: 1, CurrentStampNumber: 3}
	seed.Buttons[0].TodayCount = 7
	seed.Buttons[0].TotalCount = 19
	seed.Buttons[0].DrawTime = time.Now().Add(-24 * time.Hour).UnixMilli()
	if err := opuser.UpsertUserGachaInfo(context.Background(), testConfig.TestUserSuid, seed); err != nil {
		t.Fatal(err)
	}
	expected := proto.Clone(seed).(*papi.GachaInfo)
	expected.Buttons[0].TodayCount = 0
	assertReconciled := func() {
		t.Helper()
		response := profileCall(t, client.Enter, &papi.HomeEnterRequest{})
		var actual *papi.GachaInfo
		for _, candidate := range response.GetGachaInfos() {
			if candidate.GetId() == expected.GetId() {
				actual = candidate
				break
			}
		}
		if !proto.Equal(actual, expected) {
			t.Fatalf("Home changed gacha fields beyond the daily reset: got=%v want=%v", actual, expected)
		}
		stored, err := opuser.FindUserGachaInfo(context.Background(), testConfig.TestUserSuid, expected.GetId())
		if err != nil || !proto.Equal(stored, expected) {
			t.Fatalf("Home did not persist the gacha reset: got=%v want=%v err=%v", stored, expected, err)
		}
	}
	assertReconciled()
	assertReconciled()

	// A draw made today must survive the next Home reconciliation unchanged.
	expected.Buttons[0].DrawTime = time.Now().UnixMilli()
	expected.Buttons[0].TodayCount = 2
	expected.Buttons[0].TotalCount += 2
	if err := opuser.UpsertUserGachaInfo(context.Background(), testConfig.TestUserSuid, expected); err != nil {
		t.Fatal(err)
	}
	assertReconciled()
}
