package salary

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

var jst = time.FixedZone("JST", 9*60*60)

func NewState(suid string, now time.Time) *umodel.UserSalary {
	now = now.In(jst)
	return &umodel.UserSalary{
		SUID: suid, YearMonth: int32(now.Year()*100 + int(now.Month())),
		Payslip:              &ptransaction.Payslip{Year: int32(now.Year()), Month: int32(now.Month())},
		InitialCharacterFans: make(map[string]int64),
	}
}

// MeasurementEnd is midnight at the start of the excluded final calendar day.
func MeasurementEnd(year, month int32) time.Time {
	return time.Date(int(year), time.Month(month)+1, 0, 0, 0, 0, 0, jst)
}

func PayTime(year, month int32) time.Time {
	return MeasurementEnd(year, month).Add(12 * time.Hour)
}

// RecordAttendance records one login per eligible JST calendar day and takes a
// fresh snapshot of the supported game metrics. Run it inside the caller's
// transaction so a failed login cannot leave attendance behind.
func RecordAttendance(ctx context.Context, suid string, now time.Time) error {
	if err := CloseMeasurements(ctx, suid, now); err != nil {
		return err
	}
	now = now.In(jst)
	if !now.Before(MeasurementEnd(int32(now.Year()), int32(now.Month()))) {
		return nil
	}
	state, err := findOrNew(ctx, suid, now)
	if err != nil {
		return err
	}
	if state.PaidAt != 0 || state.ClosedAt != 0 {
		return nil
	}
	day := now.Format("2006-01-02")
	found := false
	for _, previous := range state.Payslip.GetLoginTimes() {
		if time.UnixMilli(previous).In(jst).Format("2006-01-02") == day {
			found = true
			break
		}
	}
	if !found {
		state.Payslip.LoginTimes = append(state.Payslip.LoginTimes, now.UnixMilli())
		sort.Slice(state.Payslip.LoginTimes, func(i, j int) bool { return state.Payslip.LoginTimes[i] < state.Payslip.LoginTimes[j] })
	}
	if err := snapshot(ctx, suid, state, now); err != nil {
		return err
	}
	return opuser.UpsertUserSalary(ctx, state)
}

// RefreshSnapshot updates the current month's metrics without inventing a login
// day. Closed measurement periods and paid records are never recalculated from
// newer user data.
func RefreshSnapshot(ctx context.Context, suid string, now time.Time) error {
	if err := CloseMeasurements(ctx, suid, now); err != nil {
		return err
	}
	now = now.In(jst)
	if !now.Before(MeasurementEnd(int32(now.Year()), int32(now.Month()))) {
		return nil
	}
	state, err := findOrNew(ctx, suid, now)
	if err != nil {
		return err
	}
	if state.PaidAt != 0 || state.ClosedAt != 0 {
		return nil
	}
	if err := snapshot(ctx, suid, state, now); err != nil {
		return err
	}
	return opuser.UpsertUserSalary(ctx, state)
}

// CloseMeasurements freezes the last snapshot recorded during the eligible
// period. Reading live user metrics here would leak excluded-day changes when
// the user returns late. Eligible snapshots are recorded by Home, Company and
// completed Activity calls; historical metrics cannot be reconstructed later.
func CloseMeasurements(ctx context.Context, suid string, now time.Time) error {
	states, err := opuser.FindAllUserSalaries(ctx, suid)
	if err != nil {
		return err
	}
	for _, state := range states {
		p := state.Payslip
		cutoff := MeasurementEnd(p.GetYear(), p.GetMonth())
		if state.PaidAt != 0 || state.ClosedAt != 0 || now.Before(cutoff) {
			continue
		}
		if err := creditActiveWork(ctx, suid, state, cutoff.UnixMilli()); err != nil {
			return err
		}
		p.AbsenceDays = max(0, int32(cutoff.Day())-p.GetLoginDays())
		p.ActivityHours = int32(min(int64(math.MaxInt32), state.ActivityMillis/int64(time.Hour/time.Millisecond)))
		if err := Calculate(ctx, p); err != nil {
			return err
		}
		state.ClosedAt = cutoff.UnixMilli()
		if err := opuser.UpsertUserSalary(ctx, state); err != nil {
			return err
		}
	}
	return nil
}

func findOrNew(ctx context.Context, suid string, now time.Time) (*umodel.UserSalary, error) {
	state := NewState(suid, now)
	found, err := opuser.FindUserSalary(ctx, suid, state.YearMonth)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return state, nil
	}
	return found, err
}

func snapshot(ctx context.Context, suid string, state *umodel.UserSalary, now time.Time) error {
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return err
	}
	cards, err := opuser.FindAllUserCards(ctx, suid)
	if err != nil {
		return err
	}
	characters, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		return err
	}
	p := state.Payslip
	p.ManagerLevel = user.GetManagerLevel()
	p.LoginDays = int32(len(p.GetLoginTimes()))
	elapsedDays := MeasurementEnd(p.GetYear(), p.GetMonth()).Day()
	if now.Before(MeasurementEnd(p.GetYear(), p.GetMonth())) {
		elapsedDays = now.In(jst).Day()
	}
	p.AbsenceDays = max(0, int32(elapsedDays)-p.GetLoginDays())
	var totalLevel int64
	for _, card := range cards {
		// Assist is a temporary effective level, not purchased training.
		level := card.GetLevel()
		if card.GetSupported() {
			level = card.GetBaseLevel()
		}
		totalLevel += int64(max(0, level))
	}
	if totalLevel > math.MaxInt32 {
		return fmt.Errorf("salary total card level overflows")
	}
	p.TotalCardLevel = int32(totalLevel)
	p.TotalFanAmount = 0
	if state.InitialCharacterFans == nil {
		state.InitialCharacterFans = make(map[string]int64)
	}
	var mostGrowth int64 = -1
	for _, character := range characters {
		fans := int64(character.GetLightFanAmount()) + int64(character.GetMiddleFanAmount()) + int64(character.GetHeavyFanAmount())
		if fans < 0 || p.TotalFanAmount > math.MaxInt64-fans {
			return fmt.Errorf("salary fan count overflows")
		}
		p.TotalFanAmount += fans
		initial, exists := state.InitialCharacterFans[character.GetCharacterId()]
		if !exists {
			initial = fans
			state.InitialCharacterFans[character.GetCharacterId()] = fans
		}
		if growth := fans - initial; growth > mostGrowth || (growth == mostGrowth && character.GetCharacterId() < p.GetMostGrownCharacterId()) {
			mostGrowth = growth
			p.MostGrownCharacterId = character.GetCharacterId()
		}
	}
	rankings, err := opuser.FindAllUserFanEventRankings(ctx, suid)
	if err != nil {
		return err
	}
	p.TotalFanEventScore = 0
	for _, ranking := range rankings {
		if ranking.BestScore < 0 || p.TotalFanEventScore > math.MaxInt64-ranking.BestScore {
			return fmt.Errorf("salary fan event score overflows")
		}
		p.TotalFanEventScore += ranking.BestScore
	}
	hierarchy, err := opuser.FindUserHierarchy(ctx, suid)
	if err != nil {
		return err
	}
	p.HierarchyDetailGradeId = hierarchy.GetBestDetailGradeId()
	if hierarchy.GetCurrentFixedPoint() < 0 || hierarchy.GetCurrentVariablePoint() < 0 || hierarchy.GetCurrentFixedPoint() > math.MaxInt64-hierarchy.GetCurrentVariablePoint() {
		return fmt.Errorf("salary hierarchy point overflows")
	}
	p.HierarchyPoint = hierarchy.GetCurrentFixedPoint() + hierarchy.GetCurrentVariablePoint()
	p.HierarchyRank = 1
	p.ActivityHours = int32(min(int64(math.MaxInt32), state.ActivityMillis/int64(time.Hour/time.Millisecond)))
	return Calculate(ctx, p)
}

// Calculate uses the greatest master threshold reached in each salary category.
// BasicSalary is the per-login-day manager-level amount; all other bonuses are
// single monthly amounts, as corroborated by the captured 2026-08 payslip.
func Calculate(ctx context.Context, p *ptransaction.Payslip) error {
	rules, err := opmaster.FindAllSalaries(ctx)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return fmt.Errorf("salary master rules are missing")
	}
	amount := func(kind penum.SalaryDetailType, value int64) int32 {
		var bestThreshold int64 = -1
		var result int32
		for _, rule := range rules {
			if rule.GetType() == kind && rule.GetThreshold() <= value && rule.GetThreshold() > bestThreshold {
				bestThreshold, result = rule.GetThreshold(), rule.GetAmount()
			}
		}
		return result
	}
	base := int64(amount(penum.SalaryDetailType_SalaryDetailType_BasicSalary, int64(p.GetManagerLevel()))) * int64(p.GetLoginDays())
	if base < 0 || base > math.MaxInt32 {
		return fmt.Errorf("salary basic amount overflows")
	}
	p.BasicSalary = int32(base)
	p.ActivityBonus = amount(penum.SalaryDetailType_SalaryDetailType_ActivityBonus, int64(p.GetActivityHours()))
	p.GvgHighestPointBonus = amount(penum.SalaryDetailType_SalaryDetailType_GvgHighestPointBonus, p.GetGvgHighestPoint())
	p.CardLevelBonus = amount(penum.SalaryDetailType_SalaryDetailType_CardLevelBonus, int64(p.GetTotalCardLevel()))
	p.FanAmountBonus = amount(penum.SalaryDetailType_SalaryDetailType_FanAmountBonus, p.GetTotalFanAmount())
	p.FanEventScoreBonus = amount(penum.SalaryDetailType_SalaryDetailType_FanEventScoreBonus, p.GetTotalFanEventScore())
	p.ContestScoreBonus = amount(penum.SalaryDetailType_SalaryDetailType_ContestScoreBonus, p.GetTotalContestScore())
	p.HierarchyGradeBonus = amount(penum.SalaryDetailType_SalaryDetailType_HierarchyGradeBonus, p.GetHierarchyPoint())
	return nil
}

func Total(p *ptransaction.Payslip) (int32, error) {
	var total int64
	for _, amount := range []int32{p.GetBasicSalary(), p.GetActivityBonus(), p.GetGvgHighestPointBonus(), p.GetCardLevelBonus(), p.GetFanAmountBonus(), p.GetFanEventScoreBonus(), p.GetContestScoreBonus(), p.GetHierarchyGradeBonus()} {
		if amount < 0 {
			return 0, fmt.Errorf("salary amount is negative")
		}
		total += int64(amount)
	}
	if total > math.MaxInt32 {
		return 0, fmt.Errorf("salary amount overflows")
	}
	return int32(total), nil
}

func CompanyPoints(ctx context.Context, p *ptransaction.Payslip) (int32, *pmaster.CompanySalarySetting, error) {
	setting, err := opmaster.FindCompanySalarySetting(ctx)
	if err != nil {
		return 0, nil, err
	}
	if p.GetYear()*100+p.GetMonth() < setting.GetCalcActivateYear()*100+setting.GetCalcActivateMonth() {
		return 0, setting, nil
	}
	total, err := Total(p)
	if err != nil {
		return 0, nil, err
	}
	amount := int64(total) * int64(setting.GetPointCoefficientPermil()) / 1000
	if amount < 0 || amount > math.MaxInt32 {
		return 0, nil, fmt.Errorf("salary Company amount overflows")
	}
	return int32(amount), setting, nil
}

func PendingPayslip(ctx context.Context, suid string, now time.Time) (*ptransaction.Payslip, error) {
	if err := CloseMeasurements(ctx, suid, now); err != nil {
		return nil, err
	}
	states, err := opuser.FindAllUserSalaries(ctx, suid)
	if err != nil {
		return nil, err
	}
	for _, state := range states {
		p := state.Payslip
		if state.PaidAt != 0 || p.GetLoginDays() == 0 || now.Before(PayTime(p.GetYear(), p.GetMonth())) {
			continue
		}
		result := proto.Clone(p).(*ptransaction.Payslip)
		var setting *pmaster.CompanySalarySetting
		result.ArchiveCompanyPoint, setting, err = CompanyPoints(ctx, result)
		if err != nil {
			return nil, err
		}
		result.ArchiveCompanyPointCoefficientPermil = setting.GetPointCoefficientPermil()
		return result, nil
	}
	return nil, nil
}

func ThisMonthCompanyPoint(ctx context.Context, suid string, now time.Time) (int64, error) {
	now = now.In(jst)
	state, err := opuser.FindUserSalary(ctx, suid, int32(now.Year()*100+int(now.Month())))
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	amount, _, err := CompanyPoints(ctx, state.Payslip)
	return int64(amount), err
}

// RecordActivity sums time for every participating idol, apportions it to
// calendar months, and excludes each month's final day. The character count is
// a multiplier only when every idol worked for the same duration; fan events
// call this separately for each idol's individual duration.
// Call before deleting the finished run, in
// the same transaction. Closing first credits still-present work once; neither
// a late Finish nor a retry can modify an already-closed payslip.
func RecordActivity(ctx context.Context, suid string, start, end int64, characters int32) error {
	if start <= 0 || end <= start || characters <= 0 {
		return nil
	}
	now := time.Now()
	if err := CloseMeasurements(ctx, suid, now); err != nil {
		return err
	}
	for cursor := time.UnixMilli(start).In(jst); cursor.UnixMilli() < end; {
		nextMonth := time.Date(cursor.Year(), cursor.Month()+1, 1, 0, 0, 0, 0, jst)
		cutoff := MeasurementEnd(int32(cursor.Year()), int32(cursor.Month())).UnixMilli()
		elapsed := max(int64(0), min(end, cutoff)-cursor.UnixMilli())
		if elapsed > 0 && now.UnixMilli() < cutoff {
			state, err := findOrNew(ctx, suid, cursor)
			if err != nil {
				return err
			}
			if state.PaidAt == 0 && state.ClosedAt == 0 {
				if elapsed > math.MaxInt64/int64(characters) || state.ActivityMillis > math.MaxInt64-elapsed*int64(characters) {
					return fmt.Errorf("salary activity time overflows")
				}
				state.ActivityMillis += elapsed * int64(characters)
				state.Payslip.ActivityHours = int32(min(int64(math.MaxInt32), state.ActivityMillis/int64(time.Hour/time.Millisecond)))
				if err := Calculate(ctx, state.Payslip); err != nil {
					return err
				}
				if err := opuser.UpsertUserSalary(ctx, state); err != nil {
					return err
				}
			}
		}
		cursor = nextMonth
	}
	// end can precede midnight (or even the previous month) when the player
	// finishes a long-completed activity. Never treat that as permission to
	// snapshot today's user data into the old eligible period.
	return RefreshSnapshot(ctx, suid, time.Now())
}

// At the cutoff, still-active work also belongs to the closing month, even if
// the user only presses Finish after receiving that month's salary. This runs
// once at closing; later Finish calls leave the closed month unchanged.
func creditActiveWork(ctx context.Context, suid string, state *umodel.UserSalary, cutoff int64) error {
	monthStart := time.Date(int(state.Payslip.GetYear()), time.Month(state.Payslip.GetMonth()), 1, 0, 0, 0, 0, jst).UnixMilli()
	add := func(start, end int64, count int) error {
		elapsed := max(int64(0), min(cutoff, end)-max(monthStart, start))
		if count <= 0 || elapsed > math.MaxInt64/int64(count) || state.ActivityMillis > math.MaxInt64-elapsed*int64(count) {
			return fmt.Errorf("salary activity time overflows")
		}
		credit := elapsed * int64(count)
		state.ActivityMillis += credit
		return nil
	}
	promotion, err := opuser.FindUserPromotionRun(ctx, suid)
	if err == nil {
		if err := add(promotion.GetStartTime(), promotion.GetFinishTime(), len(promotion.GetCharacters())); err != nil {
			return err
		}
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	refresh, err := opuser.FindUserRefreshRun(ctx, suid)
	if err == nil {
		if err := add(refresh.GetStartTime(), refresh.GetFinishTime(), len(refresh.GetCharacters())); err != nil {
			return err
		}
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	fanEvent, err := opuser.FindUserFanEventRun(ctx, suid)
	if err == nil {
		settingID, err := config.GetGameSettingId()
		if err != nil {
			return err
		}
		setting, err := opmaster.FindSetting(ctx, settingID)
		if err != nil {
			return err
		}
		stepMillis := int64(float64(setting.GetSecondsPerActivityFanEventBackgroundStep()) * 1000 / max(1, fanEvent.GetSpeedRate()))
		for _, character := range fanEvent.GetCharacters() {
			if err := add(fanEvent.GetStartTime(), fanEvent.GetStartTime()+int64(character.GetMaxStep())*stepMillis, 1); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	return nil
}
