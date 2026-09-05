package gacha

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *GachaServer) History(ctx context.Context, req *papi.GachaHistoryRequest) (*papi.GachaHistoryResponse, error) {
	if req.GetGachaType() == penum.GachaType_GachaType_Unknown {
		return nil, status.Error(codes.InvalidArgument, "gachaType is required")
	}
	history, err := opuser.FindUserGachaHistory(ctx, viber.GetSuid(ctx).String(), req.GetGachaType())
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return &papi.GachaHistoryResponse{Histories: []*papi.GachaHistory{}, CommonResponse: viber.NewCommonResponse(nil)}, nil
		}
		return nil, err
	}
	return &papi.GachaHistoryResponse{
		Histories:      history.Histories,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
