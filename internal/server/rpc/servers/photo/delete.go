package photo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *PhotoServer) Delete(ctx context.Context, req *papi.PhotoDeleteRequest) (*papi.PhotoDeleteResponse, error) {
	ids, err := uniqueRequiredPhotoIDs(req.GetPhotoIds())
	if err != nil {
		return nil, err
	}
	setting, err := viber.PhotoSetting(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if len(ids) > int(setting.GetPhotoDeleteAmountLimit()) {
		return nil, viber.InvalidArgumentf("at most %d photos can be deleted", setting.GetPhotoDeleteAmountLimit())
	}
	suid := viber.GetSuid(ctx).String()
	assetIDs := make([]string, 0, len(ids))
	var response *papi.PhotoDeleteResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		deleted := &papi.DeletedData{}
		profile, err := opuser.FindUserProfile(ctx, suid)
		if err != nil {
			return nil, err
		}
		var rewardAmount int64
		for _, id := range ids {
			photo, err := opuser.FindUserPhoto(ctx, suid, id)
			if err != nil {
				return nil, err
			}
			if photo.GetLocked() {
				return nil, status.Errorf(codes.FailedPrecondition, "photo %q is locked", id)
			}
			if profile.GetFavoritePhotoId() == id {
				return nil, status.Errorf(codes.FailedPrecondition, "photo %q is set on the business card", id)
			}
			level, err := opmaster.FindPhotoRarityLevel(ctx, photo.GetRarity(), photo.GetLevel())
			if err != nil {
				return nil, err
			}
			rewardAmount += int64(level.GetDeleteRewardItem())
			if photo.GetImageType() == penum.PhotoImageType_PhotoImageType_User && photo.GetAssetId() != "" {
				assetIDs = append(assetIDs, photo.GetAssetId())
			}
			deleted.Photos = append(deleted.Photos, &ptransaction.UserPhoto{PhotoId: id})
		}
		if rewardAmount <= 0 || rewardAmount > math.MaxInt32 {
			return nil, fmt.Errorf("photo delete reward amount is invalid")
		}
		results, err := viber.GrantExchangeRewards(ctx, suid, &pmaster.RewardResource{
			ResourceType: penum.ResourceType_ResourceType_Item,
			ResourceId:   viber.PhotoRerollItemID,
			Amount:       int32(rewardAmount),
		}, updated)
		if err != nil {
			return nil, err
		}
		if err := viber.DetachDeletedPhotosFromDecks(ctx, suid, ids, updated); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserPhotos(ctx, suid, ids); err != nil {
			return nil, err
		}
		common := viber.NewCommonResponse(updated)
		common.DeletedData = deleted
		response = &papi.PhotoDeleteResponse{RewardResults: results, CommonResponse: common}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	removeUserPhotoImageObjects(ctx, suid, assetIDs)
	return response, nil
}

func removeUserPhotoImageObjects(ctx context.Context, suid string, assetIDs []string) {
	if len(assetIDs) == 0 {
		return
	}
	auth, err := opuser.GetUserAuthByServerId(ctx, suid)
	if err != nil {
		mcontext.GetLogger(ctx).Warnw("failed to find photo owner while cleaning image objects", "error", err)
		return
	}
	names := make([]string, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		names = append(names, filepath.ToSlash(filepath.Join("Photo", auth.PublicUserId, assetID)))
	}
	metadata, err := opuser.FindUserPhotoImageMetadataByNames(ctx, suid, names)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		mcontext.GetLogger(ctx).Warnw("failed to find photo image metadata for cleanup", "error", err)
		return
	}
	removedNames := make([]string, 0, len(metadata))
	for _, object := range metadata {
		path := filepath.Join(config.GetResourceDir(), "Photo", object.PublicUserId, filepath.Base(object.Name)+".jpg")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			mcontext.GetLogger(ctx).Warnw("failed to delete photo image file", "path", path, "error", err)
			continue
		}
		removedNames = append(removedNames, object.Name)
	}
	// Keep metadata for files that could not be removed. A later cleanup can
	// then discover and retry those objects instead of leaking an untracked file.
	if err := opuser.DeleteUserPhotoImageMetadataByNames(ctx, suid, removedNames); err != nil {
		mcontext.GetLogger(ctx).Warnw("failed to delete photo image metadata", "error", err)
	}
}
