package salary

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	payroll "github.com/vertesan/solis-server/internal/makino/salary"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *SalaryServer) GetPayslip(ctx context.Context, req *papi.SalaryGetPayslipRequest) (*papi.SalaryGetPayslipResponse, error) {
	requested := make(map[int32]bool, len(req.GetYearMonths()))
	for _, yearMonth := range req.GetYearMonths() {
		if err := validateYearMonth(yearMonth/100, yearMonth%100); err != nil {
			return nil, err
		}
		requested[yearMonth] = true
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.SalaryGetPayslipResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := payroll.RefreshSnapshot(ctx, suid, time.Now()); err != nil {
			return nil, err
		}
		states, err := opuser.FindAllUserSalaries(ctx, suid)
		if err != nil {
			return nil, err
		}
		payslips := make([]*ptransaction.Payslip, 0, len(states))
		for _, state := range states {
			if len(requested) == 0 || requested[state.YearMonth] {
				payslips = append(payslips, state.Payslip)
			}
		}
		response = &papi.SalaryGetPayslipResponse{Payslips: payslips, CommonResponse: viber.NewCommonResponse(nil)}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
