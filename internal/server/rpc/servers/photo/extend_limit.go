package photo

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *PhotoServer) ExtendLimit(ctx context.Context, _ *papi.Empty) (*papi.PhotoExtendLimitResponse, error) {
	suid := viber.GetSuid(ctx).String()
	setting, err := viber.PhotoSetting(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	var response *papi.PhotoExtendLimitResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		if user.GetPhotoLimitExtendedCount() >= setting.GetPhotoLimitExtendLimit() {
			return nil, status.Error(codes.FailedPrecondition, "photo capacity is already fully extended")
		}
		if setting.GetPhotoLimitExtendStone() <= 0 || setting.GetPhotoLimitExtendAmount() <= 0 {
			return nil, fmt.Errorf("photo limit extension master data is invalid")
		}
		updated := &papi.UpdatedData{}
		if err := viber.ConsumeExchangeResource(
			ctx, suid, penum.ResourceType_ResourceType_Stone, "", int64(setting.GetPhotoLimitExtendStone()), updated,
		); err != nil {
			return nil, err
		}
		user.PhotoLimitExtendedCount++
		if err := opuser.UpsertUser(ctx, suid, user); err != nil {
			return nil, err
		}
		updated.User = user
		response = &papi.PhotoExtendLimitResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
