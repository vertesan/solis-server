package photo

import (
	"context"
	"strings"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) Update(ctx context.Context, req *papi.PhotoUpdateRequest) (*papi.PhotoUpdateResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if req.GetPhotoId() == "" || name == "" {
		return nil, viber.InvalidArgumentf("photoId and name are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoUpdateResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		photo, err := opuser.FindUserPhoto(ctx, suid, req.GetPhotoId())
		if err != nil {
			return nil, err
		}
		photo.Name = name
		if err := opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
			return nil, err
		}
		response = &papi.PhotoUpdateResponse{
			CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{Photos: []*ptransaction.UserPhoto{photo}}),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
