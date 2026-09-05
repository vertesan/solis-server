package showcase

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func (s *ShowcaseServer) ApplyMyset(ctx context.Context, req *papi.ShowcaseApplyMysetRequest) (*papi.ShowcaseApplyMysetResponse, error) {
	if !validShowcaseMysetNumber(req.GetNumber()) {
		return nil, viber.InvalidArgumentf("valid number is required")
	}
	suid := viber.GetSuid(ctx).String()
	myset, err := opuser.FindUserShowcaseMyset(ctx, suid, req.GetNumber())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if !myset.GetIsSave() || myset.GetShowcaseFrameId() == "" {
		return nil, status.Error(codes.FailedPrecondition, "showcase myset is not saved")
	}
	layout, err := opuser.FindUserShowcaseMysetLayout(ctx, suid, req.GetNumber())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.ShowcaseApplyMysetResponse{
		FrameId:        myset.GetShowcaseFrameId(),
		LayoutInfo:     proto.Clone(layout).(*papi.ShowcaseLayoutInfo),
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
