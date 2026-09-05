package viber

import (
	"math"

	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// CurrentStaffEnhanceCount is the number of levels trained since the baseline.
// Potential-rank increases do not count, and a reset naturally brings this back
// to zero. UserTotalCount.StaffTrainCount is a separate historical high-water.
func CurrentStaffEnhanceCount(staffs []*ptransaction.UserStaff) (int32, error) {
	seen := make(map[penum.ParameterType]struct{}, len(staffs))
	var count int64
	for _, staff := range staffs {
		if staff == nil || staff.GetParameterType() == penum.ParameterType_ParameterType_Unknown {
			return 0, FailedPreconditionf("user staff state is invalid")
		}
		if _, exists := seen[staff.GetParameterType()]; exists {
			return 0, FailedPreconditionf("user staff parameter %s is duplicated", staff.GetParameterType())
		}
		seen[staff.GetParameterType()] = struct{}{}
		if staff.GetLevel() < 1 {
			return 0, FailedPreconditionf("user staff level is invalid")
		}
		count += int64(staff.GetLevel() - 1)
		if count > math.MaxInt32 {
			return 0, FailedPreconditionf("staff enhance count overflows")
		}
	}
	return int32(count), nil
}
