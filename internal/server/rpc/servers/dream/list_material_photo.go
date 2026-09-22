package dream

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	mdream "github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sort"
)

func (s *DreamServer) ListMaterialPhoto(ctx context.Context, req *papi.DreamListMaterialPhotoRequest) (*papi.DreamListMaterialPhotoResponse, error) {
	if req.GetBasePhotoId() == "" {
		return nil, viber.InvalidArgumentf("basePhotoId is required")
	}
	suid := viber.GetSuid(ctx).String()
	if err := viber.CheckDreamUnlocked(ctx, suid); err != nil {
		return nil, viber.GameplayError(err)
	}
	base, err := opuser.FindUserPhoto(ctx, suid, req.GetBasePhotoId())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if !mdream.BaseEligible(base) || base.GetIsDreamBase() {
		return nil, status.Error(codes.FailedPrecondition, "photo cannot generate another image-training area")
	}
	photos, err := opuser.FindAllUserPhotos(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	profile, err := opuser.FindUserProfile(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	masters, err := mdream.LoadAbilities(ctx, photos)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	response := &papi.DreamListMaterialPhotoResponse{CommonResponse: viber.NewCommonResponse(nil)}
	for _, material := range photos {
		if !mdream.MaterialEligible(base, material, profile.GetFavoritePhotoId()) {
			continue
		}
		plan, err := mdream.Plan(base, material, masters)
		if err != nil {
			return nil, viber.GameplayError(err)
		}
		response.UserMaterialPhotos = append(response.UserMaterialPhotos, plan)
	}
	sort.Slice(response.UserMaterialPhotos, func(i, j int) bool {
		return response.UserMaterialPhotos[i].GetPhotoId() < response.UserMaterialPhotos[j].GetPhotoId()
	})
	return response, nil
}
