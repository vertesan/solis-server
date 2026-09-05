package showcase

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ShowcaseServer) RenameMyset(ctx context.Context, req *papi.ShowcaseRenameMysetRequest) (*papi.ShowcaseRenameMysetResponse, error) {
	if !validShowcaseMysetNumber(req.GetNumber()) || strings.TrimSpace(req.GetName()) == "" {
		return nil, viber.InvalidArgumentf("valid number and name are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ShowcaseRenameMysetResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		setting, err := opmaster.FindDefaultShowcaseSetting(ctx)
		if err != nil {
			return nil, err
		}
		if utf8.RuneCountInString(req.GetName()) > int(setting.GetShowcaseMysetNameLimit()) {
			return nil, viber.InvalidArgumentf("showcase myset name exceeds the master-data limit")
		}
		myset, err := findShowcaseMysetOrDefault(ctx, suid, req.GetNumber())
		if err != nil {
			return nil, err
		}
		myset.Name = req.GetName()
		if err := opuser.UpsertUserShowcaseMyset(ctx, suid, myset); err != nil {
			return nil, err
		}
		response = &papi.ShowcaseRenameMysetResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			ShowcaseMysets: []*ptransaction.UserShowcaseMyset{myset},
		})}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
