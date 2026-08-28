package opuser

import (
	"context"
	"fmt"
	"time"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindUserPhotoImageMetadata(ctx context.Context, name string) (*umodel.UserPhotoImageMetadata, error) {
	return ucommon.FindOne[umodel.UserPhotoImageMetadata](ctx, uname.UserPhotoImageMetadata, bson.M{"_id": name})
}

// StartUserPhotoImageUpload creates a session or replaces one whose active or
// finalizing lease is stale. A live session and a finalized object are kept.
func StartUserPhotoImageUpload(ctx context.Context, metadata *umodel.UserPhotoImageMetadata, staleBefore time.Time) (bool, error) {
	result, err := depot.User.Collection(uname.UserPhotoImageMetadata).ReplaceOne(
		ctx,
		bson.M{
			"_id": metadata.ID,
			"$or": bson.A{
				bson.M{"uploadState": bson.M{"$exists": false}},
				bson.M{
					"uploadState":          bson.M{"$in": bson.A{umodel.UserPhotoImageUploadActive, umodel.UserPhotoImageUploadFinalizing}},
					"uploadStateUpdatedAt": bson.M{"$exists": false},
				},
				bson.M{
					"uploadState":          bson.M{"$in": bson.A{umodel.UserPhotoImageUploadActive, umodel.UserPhotoImageUploadFinalizing}},
					"uploadStateUpdatedAt": bson.M{"$lte": staleBefore},
				},
			},
		},
		metadata,
		options.Replace().SetUpsert(true),
	)
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to start user photo image upload: %w", err)
	}
	return result.MatchedCount == 1 || result.UpsertedCount == 1, nil
}

// ClaimUserPhotoImageUpload atomically reserves an active session for its
// finalizer. This prevents two requests from publishing different bytes under
// the same metadata document.
func ClaimUserPhotoImageUpload(
	ctx context.Context,
	name, suid, uploadId, finalizationId string,
	stateUpdatedAt, expiresAt time.Time,
) (bool, error) {
	result, err := depot.User.Collection(uname.UserPhotoImageMetadata).UpdateOne(
		ctx,
		bson.M{
			"_id":         name,
			"SUID":        suid,
			"uploadId":    uploadId,
			"uploadState": umodel.UserPhotoImageUploadActive,
		},
		bson.M{"$set": bson.M{
			"uploadState":          umodel.UserPhotoImageUploadFinalizing,
			"finalizationId":       finalizationId,
			"uploadStateUpdatedAt": stateUpdatedAt,
			"uploadExpiresAt":      expiresAt,
		}},
	)
	if err != nil {
		return false, fmt.Errorf("failed to claim user photo image upload: %w", err)
	}
	return result.MatchedCount == 1, nil
}

func CompleteUserPhotoImageUpload(ctx context.Context, metadata *umodel.UserPhotoImageMetadata) (bool, error) {
	result, err := depot.User.Collection(uname.UserPhotoImageMetadata).ReplaceOne(
		ctx,
		bson.M{
			"_id":            metadata.ID,
			"SUID":           metadata.SUID,
			"uploadId":       metadata.UploadId,
			"uploadState":    umodel.UserPhotoImageUploadFinalizing,
			"finalizationId": metadata.FinalizationId,
		},
		metadata,
	)
	if err != nil {
		return false, fmt.Errorf("failed to complete user photo image upload: %w", err)
	}
	return result.MatchedCount == 1, nil
}

func ReleaseUserPhotoImageUpload(
	ctx context.Context,
	name, suid, uploadId, finalizationId string,
	stateUpdatedAt, expiresAt time.Time,
) (bool, error) {
	result, err := depot.User.Collection(uname.UserPhotoImageMetadata).UpdateOne(
		ctx,
		bson.M{
			"_id":            name,
			"SUID":           suid,
			"uploadId":       uploadId,
			"uploadState":    umodel.UserPhotoImageUploadFinalizing,
			"finalizationId": finalizationId,
		},
		bson.M{
			"$set": bson.M{
				"uploadState":          umodel.UserPhotoImageUploadActive,
				"uploadStateUpdatedAt": stateUpdatedAt,
				"uploadExpiresAt":      expiresAt,
			},
			"$unset": bson.M{"finalizationId": ""},
		},
	)
	if err != nil {
		return false, fmt.Errorf("failed to release user photo image upload: %w", err)
	}
	return result.MatchedCount == 1, nil
}

func DeleteExpiredUserPhotoImageUploads(ctx context.Context, now time.Time) error {
	_, err := depot.User.Collection(uname.UserPhotoImageMetadata).DeleteMany(
		ctx,
		bson.M{"uploadExpiresAt": bson.M{"$lte": now}},
	)
	if err != nil {
		return fmt.Errorf("failed to delete expired user photo image uploads: %w", err)
	}
	return nil
}
