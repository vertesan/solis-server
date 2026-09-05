package photo

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) SwitchLock(ctx context.Context, req *papi.PhotoSwitchLockRequest) (*papi.PhotoSwitchLockResponse, error) {
	ids, err := uniqueRequiredPhotoIDs(req.GetPhotoIds())
	if err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoSwitchLockResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		for _, id := range ids {
			photo, err := opuser.FindUserPhoto(ctx, suid, id)
			if err != nil {
				return nil, err
			}
			photo.Locked = !photo.GetLocked()
			if err := opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
				return nil, err
			}
			updated.Photos = append(updated.Photos, photo)
		}
		response = &papi.PhotoSwitchLockResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
