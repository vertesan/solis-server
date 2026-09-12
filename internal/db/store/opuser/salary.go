package opuser

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindUserSalary(ctx context.Context, suid string, yearMonth int32) (*umodel.UserSalary, error) {
	return ucommon.FindOne[umodel.UserSalary](ctx, uname.UserSalary, bson.M{"_id": fmt.Sprintf("%s:%06d", suid, yearMonth)})
}

func FindAllUserSalaries(ctx context.Context, suid string) ([]*umodel.UserSalary, error) {
	return ucommon.FindAll[umodel.UserSalary](ctx, uname.UserSalary, bson.M{"SUID": suid},
		options.Find().SetSort(bson.D{{Key: "yearMonth", Value: 1}}))
}

func UpsertUserSalary(ctx context.Context, state *umodel.UserSalary) error {
	if state == nil || state.SUID == "" || state.Payslip == nil || state.YearMonth != state.Payslip.GetYear()*100+state.Payslip.GetMonth() || state.Payslip.GetMonth() < 1 || state.Payslip.GetMonth() > 12 || len(state.Payslip.GetLoginTimes()) > 30 {
		return fmt.Errorf("invalid salary state")
	}
	state.Key = fmt.Sprintf("%s:%06d", state.SUID, state.YearMonth)
	return ucommon.UpsertOne(ctx, uname.UserSalary, bson.M{"_id": state.Key}, state)
}

func DeleteUserSalaries(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserSalary, bson.M{"SUID": suid})
}
