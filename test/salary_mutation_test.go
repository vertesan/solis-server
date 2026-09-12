package test

import (
	"context"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/salary"
)

// Assert directly against storage, before any Home/Company/Salary read can
// conceal a missing gameplay-side snapshot update. The final day is excluded.
func assertEligibleSalaryMutationSnapshot(t *testing.T) {
	t.Helper()
	now := time.Now().In(time.FixedZone("JST", 9*60*60))
	if !now.Before(salary.MeasurementEnd(int32(now.Year()), int32(now.Month()))) {
		t.Log("salary mutation snapshot assertion is inapplicable on the excluded final day")
		return
	}
	ctx := context.Background()
	state, err := opuser.FindUserSalary(ctx, testConfig.TestUserSuid, int32(now.Year()*100+int(now.Month())))
	if err != nil {
		t.Fatal(err)
	}
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	var levels int32
	for _, card := range cards {
		level := card.GetLevel()
		if card.GetSupported() {
			level = card.GetBaseLevel()
		}
		levels += level
	}
	if state.Payslip.GetManagerLevel() != user.GetManagerLevel() || state.Payslip.GetTotalCardLevel() != levels {
		t.Fatalf("gameplay mutation did not persist salary metrics before the next home read: manager=%d want=%d cardLevels=%d want=%d", state.Payslip.GetManagerLevel(), user.GetManagerLevel(), state.Payslip.GetTotalCardLevel(), levels)
	}
}
