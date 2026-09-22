package dream

import (
	"context"
	"errors"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	mdream "github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

func (s *DreamServer) GenerateArea(ctx context.Context, req *papi.DreamGenerateAreaRequest) (*papi.DreamGenerateAreaResponse, error) {
	if req.GetBasePhotoId() == "" || req.GetMaterialPhotoId() == "" || req.GetBasePhotoId() == req.GetMaterialPhotoId() {
		return nil, viber.InvalidArgumentf("distinct basePhotoId and materialPhotoId are required")
	}
	suid := viber.GetSuid(ctx).String()
	now := time.Now().UnixMilli()
	var response *papi.DreamGenerateAreaResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := opuser.LockUserQuestStart(ctx, suid); err != nil {
			return nil, err
		}
		if err := viber.CheckDreamUnlocked(ctx, suid); err != nil {
			return nil, err
		}
		base, err := opuser.FindUserPhoto(ctx, suid, req.GetBasePhotoId())
		if err != nil {
			return nil, err
		}
		material, err := opuser.FindUserPhoto(ctx, suid, req.GetMaterialPhotoId())
		if err != nil {
			return nil, err
		}
		profile, err := opuser.FindUserProfile(ctx, suid)
		if err != nil {
			return nil, err
		}
		if !mdream.BaseEligible(base) || base.GetIsDreamBase() || !mdream.MaterialEligible(base, material, profile.GetFavoritePhotoId()) {
			return nil, status.Error(codes.FailedPrecondition, "ineligible image-training base or material photo")
		}
		if _, err = opuser.FindUserDreamArea(ctx, suid, base.GetPhotoId()); err == nil {
			return nil, status.Error(codes.FailedPrecondition, "image-training area already exists")
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		masters, err := mdream.LoadAbilities(ctx, []*ptransaction.UserPhoto{base, material})
		if err != nil {
			return nil, err
		}
		plan, err := mdream.Plan(base, material, masters)
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		item, err := viber.ConsumePermanentItem(ctx, suid, mdream.ManualItemID, int64(plan.GetGenerateAreaItemConsumptionAmount()))
		if err != nil {
			return nil, err
		}
		updated.Items = append(updated.Items, item)
		area, err := mdream.Generate(ctx, suid, base, material, plan, now)
		if err != nil {
			return nil, err
		}
		if err = opuser.UpsertUserDreamArea(ctx, area); err != nil {
			return nil, err
		}
		if err = opuser.UpsertUserPhoto(ctx, suid, base); err != nil {
			return nil, err
		}
		if err = viber.DetachDeletedPhotosFromDecks(ctx, suid, []string{material.GetPhotoId()}, updated); err != nil {
			return nil, err
		}
		if err = opuser.DeleteUserPhoto(ctx, suid, material.GetPhotoId()); err != nil {
			return nil, err
		}
		updated.Photos = append(updated.Photos, base)
		if err = viber.AdvanceDreamMissions(ctx, suid, viber.DreamMissionProgress{Generated: 1, RankType: plan.GetAreaRankType()}, now, updated); err != nil {
			return nil, err
		}
		common := viber.NewCommonResponse(updated)
		common.DeletedData.Photos = append(common.DeletedData.Photos, &ptransaction.UserPhoto{PhotoId: material.GetPhotoId()})
		response = &papi.DreamGenerateAreaResponse{DreamUserPhoto: area.Data, CommonResponse: common}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
