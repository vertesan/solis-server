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

func FindUserPhotoImageMetadataByNames(
	ctx context.Context,
	suid string,
	names []string,
) ([]*umodel.UserPhotoImageMetadata, error) {
	if len(names) == 0 {
		return []*umodel.UserPhotoImageMetadata{}, nil
	}
	return ucommon.FindAll[umodel.UserPhotoImageMetadata](
		ctx,
		uname.UserPhotoImageMetadata,
		bson.M{"SUID": suid, "_id": bson.M{"$in": names}},
	)
}

func FindFinalizedUserPhotoImageMetadataByNames(
	ctx context.Context,
	suid string,
	names []string,
) ([]*umodel.UserPhotoImageMetadata, error) {
	if len(names) == 0 {
		return []*umodel.UserPhotoImageMetadata{}, nil
	}
	return ucommon.FindAll[umodel.UserPhotoImageMetadata](
		ctx,
		uname.UserPhotoImageMetadata,
		bson.M{
			"SUID":       suid,
			"_id":        bson.M{"$in": names},
			"generation": bson.M{"$exists": true, "$ne": ""},
		},
	)
}

func DeleteUserPhotoImageMetadataByNames(ctx context.Context, suid string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	return ucommon.Delete(
		ctx,
		uname.UserPhotoImageMetadata,
		bson.M{"SUID": suid, "_id": bson.M{"$in": names}},
	)
}

// StartUserPhotoImageUpload starts a new object or a replacement upload without
// discarding the last published generation. Live upload leases are kept.
func StartUserPhotoImageUpload(ctx context.Context, metadata *umodel.UserPhotoImageMetadata, staleBefore time.Time) (bool, error) {
	result, err := depot.User.Collection(uname.UserPhotoImageMetadata).UpdateOne(
		ctx,
		bson.M{
			"_id":          metadata.ID,
			"SUID":         metadata.SUID,
			"publicUserId": metadata.PublicUserId,
			"$or": bson.A{
				bson.M{"uploadState": bson.M{"$exists": false}},
				bson.M{"uploadState": umodel.UserPhotoImageUploadFinalized},
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
		bson.M{
			"$set": bson.M{
				"uploadId":             metadata.UploadId,
				"uploadState":          metadata.UploadState,
				"uploadStateUpdatedAt": metadata.UploadStateUpdatedAt,
				"uploadExpiresAt":      metadata.UploadExpiresAt,
				"name":                 metadata.Name,
				"bucket":               metadata.Bucket,
				"contentType":          metadata.ContentType,
			},
			"$unset": bson.M{"finalizationId": ""},
		},
		options.UpdateOne().SetUpsert(true),
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
		// Expiring an unfinished replacement must not delete the published
		// object. Its stale session can be replaced by the next upload start.
		bson.M{
			"uploadExpiresAt": bson.M{"$lte": now},
			"$or": bson.A{
				bson.M{"generation": bson.M{"$exists": false}},
				bson.M{"generation": ""},
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to delete expired user photo image uploads: %w", err)
	}
	return nil
}
