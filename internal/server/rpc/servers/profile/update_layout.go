package profile

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ProfileServer) UpdateLayout(ctx context.Context, req *papi.ProfileUpdateLayoutRequest) (*papi.ProfileUpdateLayoutResponse, error) {
	_, layoutOK := penum.ProfileLayoutType_name[int32(req.GetLayoutType())]
	_, informationOK := penum.ProfileInformationType_name[int32(req.GetInformationType())]
	_, colorOK := penum.ProfileColorType_name[int32(req.GetColorType())]
	if !layoutOK || !informationOK || !colorOK || req.GetLayoutType() == 0 || req.GetInformationType() == 0 || req.GetColorType() == 0 {
		return nil, viber.InvalidArgumentf("valid layoutType, informationType and colorType are required")
	}
	common, err := updateProfile(ctx, func(_ context.Context, _ string, profile *ptransaction.UserProfile) error {
		profile.LayoutType = req.GetLayoutType()
		profile.InformationType = req.GetInformationType()
		profile.ColorType = req.GetColorType()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &papi.ProfileUpdateLayoutResponse{CommonResponse: common}, nil
}
