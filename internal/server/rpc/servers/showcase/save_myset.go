package showcase

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/protobuf/proto"
)

func (s *ShowcaseServer) SaveMyset(ctx context.Context, req *papi.ShowcaseSaveMysetRequest) (*papi.ShowcaseSaveMysetResponse, error) {
	if !validShowcaseMysetNumber(req.GetNumber()) || req.GetFrameId() == "" || req.GetLayoutInfo() == nil {
		return nil, viber.InvalidArgumentf("valid number, frameId, and layoutInfo are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ShowcaseSaveMysetResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		frame, err := requireOwnedShowcaseFrame(ctx, suid, req.GetFrameId())
		if err != nil {
			return nil, err
		}
		if _, err := validateShowcaseLayout(ctx, suid, frame, req.GetLayoutInfo()); err != nil {
			return nil, err
		}
		myset, err := findShowcaseMysetOrDefault(ctx, suid, req.GetNumber())
		if err != nil {
			return nil, err
		}
		myset.ShowcaseFrameId = frame.GetId()
		myset.IsSave = true
		if err := opuser.UpsertUserShowcaseMyset(ctx, suid, myset); err != nil {
			return nil, err
		}
		layout := proto.Clone(req.GetLayoutInfo()).(*papi.ShowcaseLayoutInfo)
		if err := opuser.UpsertUserShowcaseMysetLayout(ctx, suid, req.GetNumber(), layout); err != nil {
			return nil, err
		}
		response = &papi.ShowcaseSaveMysetResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			ShowcaseMysets: []*ptransaction.UserShowcaseMyset{myset},
		})}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
