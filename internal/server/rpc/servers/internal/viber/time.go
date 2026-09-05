package viber

import (
	"time"
)

var JstLocation = time.FixedZone("JST", 9*60*60)

func SameJSTDay(leftMillis, rightMillis int64) bool {
	left := time.UnixMilli(leftMillis).In(JstLocation)
	right := time.UnixMilli(rightMillis).In(JstLocation)
	return left.Year() == right.Year() && left.YearDay() == right.YearDay()
}
