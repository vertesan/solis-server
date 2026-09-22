package dream

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	mdream "github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *DreamServer) ResetArea(ctx context.Context, req *papi.DreamResetAreaRequest) (*papi.DreamResetAreaResponse, error) {
	if req.GetBasePhotoId() == "" {
		return nil, viber.InvalidArgumentf("basePhotoId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.DreamResetAreaResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := opuser.LockUserQuestStart(ctx, suid); err != nil {
			return nil, err
		}
		if err := viber.CheckDreamUnlocked(ctx, suid); err != nil {
			return nil, err
		}
		area, err := opuser.FindUserDreamArea(ctx, suid, req.GetBasePhotoId())
		if err != nil {
			return nil, err
		}
		photo, err := opuser.FindUserPhoto(ctx, suid, req.GetBasePhotoId())
		if err != nil {
			return nil, err
		}
		mdream.RestoreBase(area, photo)
		if err = opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
			return nil, err
		}
		if err = opuser.DeleteUserDreamArea(ctx, suid, req.GetBasePhotoId()); err != nil {
			return nil, err
		}
		response = &papi.DreamResetAreaResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{Photos: []*ptransaction.UserPhoto{photo}})}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
