package umodel

import "github.com/vertesan/solis-server/internal/proto/ptransaction"

// UserSalary is one month's attendance and immutable paid payslip. Separate
// monthly documents keep loginTimes bounded and do not grow the user document.
// The deterministic _id also prevents duplicate monthly records under races.
type UserSalary struct {
	Key                  string                `bson:"_id"`
	SUID                 string                `bson:"SUID"`
	YearMonth            int32                 `bson:"yearMonth"`
	Payslip              *ptransaction.Payslip `bson:"payslip"`
	PaidAt               int64                 `bson:"paidAt"`
	ClosedAt             int64                 `bson:"closedAt"`
	ActivityMillis       int64                 `bson:"activityMillis"`
	InitialCharacterFans map[string]int64      `bson:"initialCharacterFans"`
}
