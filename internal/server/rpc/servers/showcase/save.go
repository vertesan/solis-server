package showcase

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func (s *ShowcaseServer) Save(ctx context.Context, req *papi.ShowcaseSaveRequest) (*papi.ShowcaseSaveResponse, error) {
	if req.GetNumber() <= 0 || req.GetFrameId() == "" || req.GetLayoutInfo() == nil {
		return nil, viber.InvalidArgumentf("number, frameId, and layoutInfo are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ShowcaseSaveResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := requireUnlockedShowcaseNumber(ctx, suid, req.GetNumber()); err != nil {
			return nil, err
		}
		frame, err := requireOwnedShowcaseFrame(ctx, suid, req.GetFrameId())
		if err != nil {
			return nil, err
		}
		toys, err := validateShowcaseLayout(ctx, suid, frame, req.GetLayoutInfo())
		if err != nil {
			return nil, err
		}
		showcase, err := findShowcaseOrDefault(ctx, suid, req.GetNumber())
		if err != nil {
			return nil, err
		}
		// Layout-only saves omit assetId in the official client. Keep the
		// existing thumbnail unless a replacement is explicitly supplied.
		assetID := req.GetAssetId()
		if assetID == "" {
			assetID = showcase.GetThumbnailAssetId()
		}
		if assetID == "" {
			return nil, viber.InvalidArgumentf("assetId is required for the first showcase save")
		}
		reusesThumbnail := showcase.GetThumbnailAssetId() == assetID
		showcase.ShowcaseFrameId = frame.GetId()
		showcase.ThumbnailAssetId = assetID
		showcase.HashTagIds, err = deriveShowcaseHashtags(ctx, req.GetLayoutInfo(), toys)
		if err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserShowcase(ctx, suid, showcase); err != nil {
			return nil, err
		}
		layout := proto.Clone(req.GetLayoutInfo()).(*papi.ShowcaseLayoutInfo)
		if err := opuser.UpsertUserShowcaseLayout(ctx, suid, req.GetNumber(), layout); err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{Showcases: []*ptransaction.UserShowcase{showcase}}
		saveMissions, err := viber.AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_ShowcaseSaveAchievement, 1,
		)
		if err != nil {
			return nil, err
		}
		hashtagMissions, err := viber.AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_ShowcaseSpecifiedHashtagIdSaveCount, 1,
			showcase.GetHashTagIds()...,
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = viber.AppendUniquePhotoMissions(saveMissions, hashtagMissions)
		// Editing a saved case uploads a new generation under its existing
		// thumbnail name. Only a newly selected image consumes a reservation.
		if !reusesThumbnail {
			if err := consumeShowcaseThumbnailImageID(ctx, suid, assetID, updated); err != nil {
				return nil, err
			}
		}
		response = &papi.ShowcaseSaveResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func findShowcaseOrDefault(ctx context.Context, suid string, number int32) (*ptransaction.UserShowcase, error) {
	showcase, err := opuser.FindUserShowcase(ctx, suid, number)
	if err == nil {
		return showcase, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	return &ptransaction.UserShowcase{
		Number: number,
		Name:   showcaseDefaultName(number),
		IsOpen: true,
	}, nil
}

func deriveShowcaseHashtags(
	ctx context.Context,
	layout *papi.ShowcaseLayoutInfo,
	toys map[string]*pmaster.ShowcaseToy,
) ([]string, error) {
	counts := make(map[string]int64)
	for _, asset := range layout.GetShowcaseToyAssetInfos() {
		masterToy := toys[asset.GetShowcaseToyId()]
		if masterToy == nil {
			continue
		}
		for _, hashtagID := range masterToy.GetHashTagIds() {
			counts[hashtagID] += int64(asset.GetCount())
		}
	}
	if len(counts) == 0 {
		return []string{}, nil
	}
	masters, err := opmaster.FindAllShowcaseHashtags(ctx)
	if err != nil {
		return nil, err
	}
	numbers := make(map[string]int32, len(masters))
	for _, master := range masters {
		numbers[master.GetId()] = master.GetNumber()
	}
	ids := make([]string, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if counts[ids[i]] != counts[ids[j]] {
			return counts[ids[i]] > counts[ids[j]]
		}
		left, leftExists := numbers[ids[i]]
		right, rightExists := numbers[ids[j]]
		if leftExists != rightExists {
			return leftExists
		}
		if left != right {
			return left < right
		}
		return ids[i] < ids[j]
	})
	if len(ids) > showcaseMaximumHashtagCount {
		ids = ids[:showcaseMaximumHashtagCount]
	}
	return ids, nil
}

func consumeShowcaseThumbnailImageID(ctx context.Context, suid, assetID string, updated *papi.UpdatedData) error {
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return err
	}
	reserved := make(map[string]struct{}, len(user.GetNextPhotoImageIds()))
	for _, imageID := range user.GetNextPhotoImageIds() {
		reserved[imageID] = struct{}{}
	}
	if _, exists := reserved[assetID]; !exists {
		return status.Errorf(codes.FailedPrecondition, "showcase thumbnail image ID %q is not reserved for this user", assetID)
	}
	remaining := make([]string, 0, viber.ReservedPhotoImageIDCount)
	for _, imageID := range user.GetNextPhotoImageIds() {
		if imageID != assetID {
			remaining = append(remaining, imageID)
		}
	}
	for len(remaining) < viber.ReservedPhotoImageIDCount {
		candidate := uuid.NewString()
		if _, exists := reserved[candidate]; exists {
			continue
		}
		reserved[candidate] = struct{}{}
		remaining = append(remaining, candidate)
	}
	user.NextPhotoImageIds = remaining[:viber.ReservedPhotoImageIDCount]
	if err := opuser.UpsertUser(ctx, suid, user); err != nil {
		return err
	}
	updated.User = user
	return nil
}
