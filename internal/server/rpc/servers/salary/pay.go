package salary

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	payroll "github.com/vertesan/solis-server/internal/makino/salary"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (s *SalaryServer) Pay(ctx context.Context, req *papi.SalaryPayRequest) (*papi.SalaryPayResponse, error) {
	if err := validateYearMonth(req.GetYear(), req.GetMonth()); err != nil {
		return nil, err
	}
	now := time.Now()
	if now.Before(payroll.PayTime(req.GetYear(), req.GetMonth())) {
		return nil, viber.FailedPreconditionf("salary is payable at 12:00 JST on the month's final day")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.SalaryPayResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := payroll.CloseMeasurements(ctx, suid, now); err != nil {
			return nil, err
		}
		state, err := opuser.FindUserSalary(ctx, suid, req.GetYear()*100+req.GetMonth())
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, viber.FailedPreconditionf("no attendance was recorded for that month")
		}
		if err != nil {
			return nil, err
		}
		if state.PaidAt != 0 {
			return nil, viber.FailedPreconditionf("salary was already received")
		}
		if state.Payslip.GetLoginDays() == 0 {
			return nil, viber.FailedPreconditionf("no attendance was recorded for that month")
		}
		amount, err := payroll.Total(state.Payslip)
		if err != nil {
			return nil, err
		}
		balance, err := opuser.FindUserBalance(ctx, suid)
		if err != nil {
			return nil, err
		}
		if balance.GetFreeBalance() < 0 || balance.GetFreeBalance() > math.MaxInt32-amount {
			return nil, viber.FailedPreconditionf("salary diamond balance overflows")
		}
		balance.FreeBalance += amount
		if err := opuser.UpsertUserBalance(ctx, suid, balance); err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{Balance: balance}
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		if amount > user.GetHighestSalary() {
			user.HighestSalary = amount
			if err := opuser.UpsertUser(ctx, suid, user); err != nil {
				return nil, err
			}
			updated.User = user
		}
		companyPoint, setting, err := payroll.CompanyPoints(ctx, state.Payslip)
		if err != nil {
			return nil, err
		}
		unlocked, err := viber.NewCommerceConditionEvaluator(ctx, suid, now.UnixMilli()).Satisfied(setting.GetFunctionUnlockConditionId())
		if err != nil {
			return nil, err
		}
		if !unlocked {
			companyPoint = 0
		}
		state.Payslip.ArchiveCompanyPoint = companyPoint
		if unlocked {
			state.Payslip.ArchiveCompanyPointCoefficientPermil = setting.GetPointCoefficientPermil()
		}
		if companyPoint > 0 {
			if _, err := viber.GrantCompanyPoint(ctx, suid, "company_grade_point-salaly", int64(companyPoint), updated); err != nil {
				return nil, err
			}
			if err := viber.ReconcileCompanyMissions(ctx, suid); err != nil {
				return nil, err
			}
			updated.Missions, err = opuser.FindAllUserMissions(ctx, suid)
			if err != nil {
				return nil, err
			}
		}
		state.PaidAt = now.UnixMilli()
		if err := opuser.UpsertUserSalary(ctx, state); err != nil {
			return nil, err
		}
		response = &papi.SalaryPayResponse{Payslip: state.Payslip, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
