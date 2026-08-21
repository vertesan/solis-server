package viber

import "time"

var jstLocation = time.FixedZone("JST", 9*60*60)

func SameJSTDay(leftMillis, rightMillis int64) bool {
	left := time.UnixMilli(leftMillis).In(jstLocation)
	right := time.UnixMilli(rightMillis).In(jstLocation)
	return left.Year() == right.Year() && left.YearDay() == right.YearDay()
}

// JSTHalfDayStart returns the most recent 00:00 or 12:00 JST boundary.
func JSTHalfDayStart(now time.Time) time.Time {
	local := now.In(jstLocation)
	hour := 0
	if local.Hour() >= 12 {
		hour = 12
	}
	return time.Date(local.Year(), local.Month(), local.Day(), hour, 0, 0, 0, jstLocation)
}

// NextJSTHalfDayStart returns the next 00:00 or 12:00 JST boundary.
func NextJSTHalfDayStart(now time.Time) time.Time {
	return JSTHalfDayStart(now).Add(12 * time.Hour)
}

// NextJSTDayStart returns the next 00:00 JST boundary.
func NextJSTDayStart(now time.Time) time.Time {
	local := now.In(jstLocation)
	return time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, jstLocation)
}

// NextJSTWeekStart returns the next Sunday 00:00 JST boundary.
func NextJSTWeekStart(now time.Time) time.Time {
	local := now.In(jstLocation)
	days := (7 - int(local.Weekday())) % 7
	if days == 0 {
		days = 7
	}
	return time.Date(local.Year(), local.Month(), local.Day()+days, 0, 0, 0, 0, jstLocation)
}

// NextJSTMonthStart returns the first 00:00 JST boundary of the next month.
func NextJSTMonthStart(now time.Time) time.Time {
	local := now.In(jstLocation)
	return time.Date(local.Year(), local.Month()+1, 1, 0, 0, 0, 0, jstLocation)
}

// IsJSTMarketDiscountDay reports the recurring market sale dates used after
// the finite dated conditions shipped in the client master data expire.
func IsJSTMarketDiscountDay(now time.Time) bool {
	switch now.In(jstLocation).Day() {
	case 10, 20, 30:
		return true
	default:
		return false
	}
}

// NextJSTMonthlyOccurrence advances a configured JST reset timestamp to the
// first matching monthly occurrence strictly after now.
func NextJSTMonthlyOccurrence(resetMillis, nowMillis int64) int64 {
	reset := time.UnixMilli(resetMillis).In(jstLocation)
	current := time.UnixMilli(nowMillis).In(jstLocation)
	months := (current.Year()-reset.Year())*12 + int(current.Month()-reset.Month())
	next := reset.AddDate(0, months, 0)
	if !next.After(current) {
		next = next.AddDate(0, 1, 0)
	}
	return next.UnixMilli()
}
