package test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/salary"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/http/admin"
	"github.com/vertesan/solis-server/internal/utils/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestSalaryMonthlyScenario(t *testing.T) {
	setupTestUser(t)
	callUserGod(t, &admin.UserGodRequest{ServerUserId: testConfig.TestUserSuid})
	dbCtx := context.Background()
	suid := testConfig.TestUserSuid
	if err := opuser.DeleteUserSalaries(dbCtx, suid); err != nil {
		t.Fatal(err)
	}
	user, err := opuser.FindUser(dbCtx, suid)
	if err != nil {
		t.Fatal(err)
	}
	user.HighestSalary = 0
	if err := opuser.UpsertUser(dbCtx, suid, user); err != nil {
		t.Fatal(err)
	}
	jst := time.FixedZone("JST", 9*60*60)
	now := time.Now().In(jst)
	month := time.Date(now.Year(), now.Month()-1, 1, 12, 0, 0, 0, jst)
	year, monthNumber := int32(month.Year()), int32(month.Month())
	monthKey := year*100 + monthNumber
	apply := func(fn func(context.Context) error) {
		t.Helper()
		if err := transaction.WithTransaction(dbCtx, func(ctx context.Context) (any, error) { return nil, fn(ctx) }); err != nil {
			t.Fatal(err)
		}
	}
	apply(func(ctx context.Context) error { return salary.RecordAttendance(ctx, suid, month) })
	apply(func(ctx context.Context) error { return salary.RecordAttendance(ctx, suid, month.Add(time.Hour)) })
	apply(func(ctx context.Context) error { return salary.RecordAttendance(ctx, suid, month.AddDate(0, 0, 1)) })
	state, err := opuser.FindUserSalary(dbCtx, suid, monthKey)
	if err != nil {
		t.Fatal(err)
	}
	if state.Payslip.GetLoginDays() != 2 || len(state.Payslip.GetLoginTimes()) != 2 {
		t.Fatalf("salary attendance counted repeated same-day entry: %v", state.Payslip)
	}
	assertSalaryMasterAmounts(t, state.Payslip)
	eligibleManagerLevel := state.Payslip.GetManagerLevel()
	// Simulate an excluded-day mutation before the very first post-cutoff
	// salary read. Closing must keep the last eligible snapshot, not today's
	// live user state.
	user.ManagerLevel = 1
	if err := opuser.UpsertUser(dbCtx, suid, user); err != nil {
		t.Fatal(err)
	}

	// A still-running refresh contributes only its three idol-hours before the
	// cutoff, without requiring Finish before receiving salary.
	cutoff := salary.MeasurementEnd(year, monthNumber)
	if err := opuser.UpsertUserRefreshRun(dbCtx, suid, &papi.RefreshProgress{
		ActivityRefreshId: "salary-test-refresh", StartTime: cutoff.Add(-time.Hour).UnixMilli(), FinishTime: cutoff.Add(time.Hour).UnixMilli(),
		Characters: []*papi.ActivityCharacterInfo{{CharacterId: "char-kot"}, {CharacterId: "char-skr"}, {CharacterId: "char-rei"}},
	}); err != nil {
		t.Fatal(err)
	}
	apply(func(ctx context.Context) error { return salary.RecordAttendance(ctx, suid, cutoff.Add(time.Minute)) })
	state, err = opuser.FindUserSalary(dbCtx, suid, monthKey)
	if err != nil {
		t.Fatal(err)
	}
	if state.Payslip.GetLoginDays() != 2 || state.Payslip.GetManagerLevel() != eligibleManagerLevel || state.ActivityMillis != int64(3*time.Hour/time.Millisecond) || state.ClosedAt != cutoff.UnixMilli() {
		t.Fatalf("closing salary included final-day attendance or lost running work: %+v", state)
	}
	frozen := proto.Clone(state.Payslip).(*ptransaction.Payslip)
	user.ManagerLevel = 2
	if err := opuser.UpsertUser(dbCtx, suid, user); err != nil {
		t.Fatal(err)
	}
	apply(func(ctx context.Context) error { return salary.RecordAttendance(ctx, suid, cutoff.Add(20*time.Hour)) })
	state, err = opuser.FindUserSalary(dbCtx, suid, monthKey)
	if err != nil || !proto.Equal(frozen, state.Payslip) {
		t.Fatalf("last-day gameplay changed closed salary: state=%v err=%v", state, err)
	}
	beforePay, err := salary.PendingPayslip(dbCtx, suid, salary.PayTime(year, monthNumber).Add(-time.Nanosecond))
	if err != nil || beforePay != nil {
		t.Fatalf("salary became available before final day 12:00 JST: %v %v", beforePay, err)
	}
	due, err := salary.PendingPayslip(dbCtx, suid, salary.PayTime(year, monthNumber))
	if err != nil || due == nil {
		t.Fatalf("salary unavailable at final day 12:00 JST: %v %v", due, err)
	}
	if err := opuser.DeleteUserRefreshRun(dbCtx, suid); err != nil {
		t.Fatal(err)
	}
	apply(func(ctx context.Context) error {
		return salary.RecordActivity(ctx, suid, cutoff.Add(-time.Hour).UnixMilli(), cutoff.Add(time.Hour).UnixMilli(), 3)
	})
	state, err = opuser.FindUserSalary(dbCtx, suid, monthKey)
	if err != nil || state.ActivityMillis != int64(3*time.Hour/time.Millisecond) {
		t.Fatalf("finishing already-accounted work counted hours twice: %+v %v", state, err)
	}
	amount, err := salary.Total(state.Payslip)
	if err != nil || amount <= 0 {
		t.Fatalf("expected positive master-based salary, got %d %v", amount, err)
	}
	companyAmount, _, err := salary.CompanyPoints(dbCtx, state.Payslip)
	if err != nil {
		t.Fatal(err)
	}
	balanceBefore, err := opuser.FindUserBalance(dbCtx, suid)
	if err != nil {
		t.Fatal(err)
	}
	companyBefore, err := opuser.FindUserCompany(dbCtx, suid)
	if err != nil {
		t.Fatal(err)
	}
	client := papi.NewSalaryClient(testConfig.GrpcConn)
	home := hierarchyGuildCall(t, func(ctx context.Context) (*papi.HomeEnterResponse, error) {
		return papi.NewHomeClient(testConfig.GrpcConn).Enter(ctx, &papi.HomeEnterRequest{})
	})
	if home.GetPayslip().GetYear() != year || home.GetPayslip().GetMonth() != monthNumber {
		t.Fatalf("Home.Enter missing due salary: %v", home.GetPayslip())
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()
			_, err := client.Pay(ctx, &papi.SalaryPayRequest{Year: year, Month: monthNumber})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	var succeeded, rejected int
	for err := range results {
		if err == nil {
			succeeded++
		} else if status.Code(err) == codes.FailedPrecondition {
			rejected++
		} else {
			t.Fatalf("unexpected salary payment failure: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("monthly salary race: success=%d rejected=%d", succeeded, rejected)
	}
	balanceAfter, err := opuser.FindUserBalance(dbCtx, suid)
	if err != nil {
		t.Fatal(err)
	}
	companyAfter, err := opuser.FindUserCompany(dbCtx, suid)
	if err != nil {
		t.Fatal(err)
	}
	if balanceAfter.GetFreeBalance()-balanceBefore.GetFreeBalance() != amount || balanceAfter.GetPaidBalance() != balanceBefore.GetPaidBalance() {
		t.Fatalf("salary diamonds not granted once: before=%v after=%v amount=%d", balanceBefore, balanceAfter, amount)
	}
	if companyAfter.GetSalaryPoint()-companyBefore.GetSalaryPoint() != int64(companyAmount) {
		t.Fatalf("salary Company points delta = %d, want %d", companyAfter.GetSalaryPoint()-companyBefore.GetSalaryPoint(), companyAmount)
	}
	userAfter, err := opuser.FindUser(dbCtx, suid)
	if err != nil || userAfter.GetHighestSalary() != amount {
		t.Fatalf("highest salary not persisted: %v %v", userAfter, err)
	}
	get := hierarchyGuildCall(t, func(ctx context.Context) (*papi.SalaryGetPayslipResponse, error) {
		return client.GetPayslip(ctx, &papi.SalaryGetPayslipRequest{YearMonths: []int32{monthKey}})
	})
	if len(get.GetPayslips()) != 1 || get.GetPayslips()[0].GetArchiveCompanyPoint() != companyAmount || get.GetCommonResponse() == nil {
		t.Fatalf("paid salary archive missing: %v", get)
	}
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Pay(ctx, &papi.SalaryPayRequest{Year: int32(now.Year() + 1), Month: 1})
	cancel()
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("future salary accepted: %v", err)
	}
	if again, err := salary.PendingPayslip(dbCtx, suid, time.Now()); err != nil || again != nil {
		t.Fatalf("paid salary still pending: %v %v", again, err)
	}
}

func TestSalaryLateActivityFinishScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	suid := testConfig.TestUserSuid
	if err := opuser.DeleteUserSalaries(ctx, suid); err != nil {
		t.Fatal(err)
	}
	jst := time.FixedZone("JST", 9*60*60)
	now := time.Now().In(jst)
	month := time.Date(now.Year(), now.Month()-1, 1, 12, 0, 0, 0, jst)
	year, monthNumber := int32(month.Year()), int32(month.Month())
	monthKey := year*100 + monthNumber
	if err := salary.RecordAttendance(ctx, suid, month); err != nil {
		t.Fatal(err)
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		t.Fatal(err)
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		t.Fatal(err)
	}
	stepMillis := int64(setting.GetSecondsPerActivityFanEventBackgroundStep()) * 1000
	if stepMillis <= 0 {
		t.Fatal("fan event master background step duration is invalid")
	}
	cutoff := salary.MeasurementEnd(year, monthNumber)
	start, end := cutoff.Add(-2*time.Hour).UnixMilli(), cutoff.Add(time.Hour).UnixMilli()
	steps := int32((end - start + stepMillis - 1) / stepMillis)
	progress := &papi.FanEventProgress{
		ActivityFanEventId: "salary-test-fan-event", StartTime: start, SpeedRate: 1,
		Characters: []*papi.FanEventCharacterInfo{
			{CharacterId: "char-kot", MaxStep: steps, CompletedStep: steps},
			{CharacterId: "char-skr", MaxStep: steps, CompletedStep: steps},
			{CharacterId: "char-rei", MaxStep: steps, CompletedStep: steps},
		},
	}
	if err := opuser.UpsertUserFanEventRun(ctx, suid, progress); err != nil {
		t.Fatal(err)
	}
	// No Home/Salary call has closed this month yet. Mirror FinishFanEvent's
	// per-character accounting while the run is still stored, then delete it.
	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		for range progress.GetCharacters() {
			if err := salary.RecordActivity(ctx, suid, start, end, 1); err != nil {
				return nil, err
			}
		}
		return nil, opuser.DeleteUserFanEventRun(ctx, suid)
	}); err != nil {
		t.Fatal(err)
	}
	state, err := opuser.FindUserSalary(ctx, suid, monthKey)
	if err != nil {
		t.Fatal(err)
	}
	wantMillis := int64(6 * time.Hour / time.Millisecond)
	if state.ClosedAt != cutoff.UnixMilli() || state.ActivityMillis != wantMillis || state.Payslip.GetActivityHours() != 6 {
		t.Fatalf("first late Finish double-counted active work: %+v, want six idol-hours", state)
	}
	frozen := proto.Clone(state.Payslip).(*ptransaction.Payslip)
	// A closed but not-yet-paid period is as immutable as a paid one. A later
	// Finish must not alter its accounting, even if its timestamps overlap it.
	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return nil, salary.RecordActivity(ctx, suid, start, end, 3)
	}); err != nil {
		t.Fatal(err)
	}
	after, err := opuser.FindUserSalary(ctx, suid, monthKey)
	if err != nil || after.ActivityMillis != wantMillis || !proto.Equal(frozen, after.Payslip) {
		t.Fatalf("late activity mutated a closed unpaid payslip: %+v %v", after, err)
	}
}

func TestSalaryAllIdolWorkDurationScenario(t *testing.T) {
	for _, simultaneous := range []bool{false, true} {
		name := "100h_plus_150h"
		if simultaneous {
			name = "all_simultaneous_activity_kinds"
		}
		t.Run(name, func(t *testing.T) {
			setupTestUser(t)
			ctx := context.Background()
			suid := testConfig.TestUserSuid
			if err := opuser.DeleteUserSalaries(ctx, suid); err != nil {
				t.Fatal(err)
			}
			jst := time.FixedZone("JST", 9*60*60)
			now := time.Now().In(jst)
			month := time.Date(now.Year(), now.Month()-1, 1, 12, 0, 0, 0, jst)
			year, monthNumber := int32(month.Year()), int32(month.Month())
			monthKey := year*100 + monthNumber
			if err := salary.RecordAttendance(ctx, suid, month); err != nil {
				t.Fatal(err)
			}
			settingID, err := config.GetGameSettingId()
			if err != nil {
				t.Fatal(err)
			}
			setting, err := opmaster.FindSetting(ctx, settingID)
			if err != nil {
				t.Fatal(err)
			}
			stepMillis := int64(setting.GetSecondsPerActivityFanEventBackgroundStep()) * 1000
			if stepMillis <= 0 {
				t.Fatal("fan event master background step duration is invalid")
			}
			steps := func(duration time.Duration) int32 {
				t.Helper()
				millis := int64(duration / time.Millisecond)
				if millis%stepMillis != 0 {
					t.Skip("master step duration cannot represent the exact 100h/150h salary example")
				}
				return int32(millis / stepMillis)
			}
			cutoff := salary.MeasurementEnd(year, monthNumber)
			// One idol stops after 100 hours, the other after 150 hours. They
			// overlap in real time but must contribute 250, not 150 or 300.
			if err := opuser.UpsertUserFanEventRun(ctx, suid, &papi.FanEventProgress{
				ActivityFanEventId: "salary-test-independent-durations",
				StartTime:          cutoff.Add(-200 * time.Hour).UnixMilli(), SpeedRate: 1,
				Characters: []*papi.FanEventCharacterInfo{
					{CharacterId: "char-kot", MaxStep: steps(100 * time.Hour)},
					{CharacterId: "char-skr", MaxStep: steps(150 * time.Hour)},
				},
			}); err != nil {
				t.Fatal(err)
			}
			wantHours := int32(250)
			if simultaneous {
				// Concurrent promotion and refresh contribute all three idols
				// apiece; the part after the measurement cutoff is excluded.
				if err := opuser.UpsertUserPromotionRun(ctx, suid, &papi.PromotionProgress{
					ActivityPromotionId: "salary-test-promotion",
					StartTime:           cutoff.Add(-10 * time.Hour).UnixMilli(), FinishTime: cutoff.Add(time.Hour).UnixMilli(),
					Characters: []*papi.ActivityCharacterInfo{{CharacterId: "char-rei"}, {CharacterId: "char-ngs"}, {CharacterId: "char-mei"}},
				}); err != nil {
					t.Fatal(err)
				}
				if err := opuser.UpsertUserRefreshRun(ctx, suid, &papi.RefreshProgress{
					ActivityRefreshId: "salary-test-refresh",
					StartTime:         cutoff.Add(-20 * time.Hour).UnixMilli(), FinishTime: cutoff.Add(time.Hour).UnixMilli(),
					Characters: []*papi.ActivityCharacterInfo{{CharacterId: "char-kkr"}, {CharacterId: "char-szk"}, {CharacterId: "char-suz"}},
				}); err != nil {
					t.Fatal(err)
				}
				wantHours += 10*3 + 20*3
			}
			client := papi.NewSalaryClient(testConfig.GrpcConn)
			response := hierarchyGuildCall(t, func(ctx context.Context) (*papi.SalaryGetPayslipResponse, error) {
				return client.GetPayslip(ctx, &papi.SalaryGetPayslipRequest{YearMonths: []int32{monthKey}})
			})
			if len(response.GetPayslips()) != 1 || response.GetPayslips()[0].GetActivityHours() != wantHours {
				t.Fatalf("salary did not sum all idol durations: %v, want %d hours", response.GetPayslips(), wantHours)
			}
			state, err := opuser.FindUserSalary(ctx, suid, monthKey)
			if err != nil {
				t.Fatal(err)
			}
			if state.ActivityMillis != int64(wantHours)*int64(time.Hour/time.Millisecond) || state.ClosedAt != cutoff.UnixMilli() {
				t.Fatalf("summed duration was not persisted at the cutoff: %+v", state)
			}
			assertSalaryMasterAmounts(t, state.Payslip)
		})
	}
}

func assertSalaryMasterAmounts(t *testing.T, payslip *ptransaction.Payslip) {
	t.Helper()
	rules, err := opmaster.FindAllSalaries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		kind       penum.SalaryDetailType
		value      int64
		actual     int32
		multiplier int32
	}{
		{penum.SalaryDetailType_SalaryDetailType_BasicSalary, int64(payslip.GetManagerLevel()), payslip.GetBasicSalary(), payslip.GetLoginDays()},
		{penum.SalaryDetailType_SalaryDetailType_ActivityBonus, int64(payslip.GetActivityHours()), payslip.GetActivityBonus(), 1},
		{penum.SalaryDetailType_SalaryDetailType_CardLevelBonus, int64(payslip.GetTotalCardLevel()), payslip.GetCardLevelBonus(), 1},
		{penum.SalaryDetailType_SalaryDetailType_FanAmountBonus, payslip.GetTotalFanAmount(), payslip.GetFanAmountBonus(), 1},
		{penum.SalaryDetailType_SalaryDetailType_HierarchyGradeBonus, payslip.GetHierarchyPoint(), payslip.GetHierarchyGradeBonus(), 1},
	} {
		var bestThreshold int64 = -1
		var expected int32
		for _, rule := range rules {
			if rule.GetType() == check.kind && rule.GetThreshold() <= check.value && rule.GetThreshold() > bestThreshold {
				expected = rule.GetAmount() * check.multiplier
				bestThreshold = rule.GetThreshold()
			}
		}
		if check.actual != expected {
			t.Fatalf("salary %s = %d, master requires %d", check.kind, check.actual, expected)
		}
	}
}
