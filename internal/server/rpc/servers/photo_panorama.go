package servers

import (
	"context"
	"errors"
	"slices"
	"sort"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/mcontext"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	photoPanoramaEventName                   = "360°LIVE "
	photoPanoramaShootingTutorialConditionID = "cd-tutorial-finish-photo-create-shooting"
)

type PhotoPanoramaServer struct {
	papi.UnimplementedPhotoPanoramaServer
}

type resolvedPhotoPanoramaMusic struct {
	info  *papi.PhotoPanoramaMusic
	music *pmaster.Music
	stage *pmaster.Stage
}

func (s *PhotoPanoramaServer) List(
	ctx context.Context,
	_ *papi.Empty,
) (*papi.PhotoPanoramaListResponse, error) {
	suid := viber.GetSuid(ctx).String()
	resolved, err := listPhotoPanoramaMusics(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	musics := make([]*papi.PhotoPanoramaMusic, 0, len(resolved))
	unlockStates := make(map[string]bool)
	for _, value := range resolved {
		conditionID := value.info.GetUnlockConditionId()
		unlocked, exists := unlockStates[conditionID]
		if !exists {
			unlocked, err = photoPanoramaUnlockConditionSatisfied(ctx, suid, conditionID)
			if err != nil {
				return nil, viber.GameplayError(err)
			}
			unlockStates[conditionID] = unlocked
		}
		value.info.IsUnlocked = unlocked
		musics = append(musics, value.info)
	}
	return &papi.PhotoPanoramaListResponse{
		Musics:         musics,
		DokanInfos:     []*papi.DokanInfo{},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *PhotoPanoramaServer) CheckShooting(
	ctx context.Context,
	req *papi.PhotoPanoramaCheckShootingRequest,
) (*papi.PhotoPanoramaCheckShootingResponse, error) {
	if _, err := checkPhotoPanoramaShooting(ctx, req); err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.PhotoPanoramaCheckShootingResponse{
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *PhotoPanoramaServer) CreateShootings(
	ctx context.Context,
	req *papi.PhotoPanoramaCreateShootingsRequest,
) (*papi.PhotoPanoramaCreateShootingsResponse, error) {
	if len(req.GetCreateShootingParams()) == 0 {
		return nil, viber.InvalidArgumentf("at least one createShootingParam is required")
	}
	if len(req.GetCameraTypes()) != len(req.GetCreateShootingParams()) {
		return nil, viber.InvalidArgumentf("cameraTypes and createShootingParams must have equal lengths")
	}
	for _, cameraType := range req.GetCameraTypes() {
		if !validPhotoPanoramaCameraType(cameraType) {
			return nil, viber.InvalidArgumentf("unsupported panorama camera type %d", cameraType)
		}
	}

	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoPanoramaCreateShootingsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		resolved, err := checkPhotoPanoramaShooting(ctx, &papi.PhotoPanoramaCheckShootingRequest{
			PhotoMusicId: req.GetPhotoMusicId(),
			CharacterIds: req.GetCharacterIds(),
			CostumeIds:   req.GetCostumeIds(),
			HairIds:      req.GetHairIds(),
		})
		if err != nil {
			return nil, err
		}
		if req.GetStartCharacterId() != "" && !slices.Contains(req.GetCharacterIds(), req.GetStartCharacterId()) {
			return nil, viber.InvalidArgumentf("startCharacterId must be one of characterIds")
		}
		if !photoPanoramaCameraPositionExists(resolved.stage, req.GetStartCameraPositionIndex()) {
			return nil, viber.InvalidArgumentf(
				"stage %q has no panorama camera position %d",
				resolved.stage.GetId(), req.GetStartCameraPositionIndex(),
			)
		}
		for _, param := range req.GetCreateShootingParams() {
			if param == nil {
				return nil, viber.InvalidArgumentf("createShootingParam is required")
			}
			if param.GetMainCharacterId() != "" && !slices.Contains(req.GetCharacterIds(), param.GetMainCharacterId()) {
				return nil, viber.InvalidArgumentf("mainCharacterId must be one of characterIds")
			}
		}

		result, err := createUserPhotoShootings(
			ctx,
			suid,
			photoShootingScene{
				placeName:             resolved.stage.GetName(),
				eventName:             photoPanoramaEventName,
				candidateCharacterIDs: slices.Clone(req.GetCharacterIds()),
				panoramaShooting:      true,
				musicID:               req.GetPhotoMusicId(),
			},
			commonPhotoParams(req.GetCreateShootingParams()),
			req.GetManualCount(),
		)
		if err != nil {
			return nil, err
		}
		response = &papi.PhotoPanoramaCreateShootingsResponse{
			Photos:         result.photos,
			IsGift:         result.isGift,
			CommonResponse: viber.NewCommonResponse(result.updated),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func checkPhotoPanoramaShooting(
	ctx context.Context,
	req *papi.PhotoPanoramaCheckShootingRequest,
) (*resolvedPhotoPanoramaMusic, error) {
	if req.GetPhotoMusicId() == "" {
		return nil, viber.InvalidArgumentf("photoMusicId is required")
	}
	if err := validatePhotoStyleLists(req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds()); err != nil {
		return nil, err
	}
	if hasDuplicatePhotoPanoramaCharacters(req.GetCharacterIds()) {
		return nil, viber.InvalidArgumentf("characterIds must be unique")
	}

	resolved, err := resolvePhotoPanoramaMusic(ctx, req.GetPhotoMusicId())
	if err != nil {
		return nil, err
	}
	wantCharacters := int(resolved.music.GetDisplayCharacterAmount())
	if wantCharacters <= 0 {
		return nil, viber.FailedPreconditionf("panorama music %q has no valid cast size", req.GetPhotoMusicId())
	}
	if len(req.GetCharacterIds()) != wantCharacters {
		return nil, viber.InvalidArgumentf(
			"panorama music %q requires %d characters", req.GetPhotoMusicId(), wantCharacters,
		)
	}

	suid := viber.GetSuid(ctx).String()
	unlocked, err := photoPanoramaUnlockConditionSatisfied(
		ctx, suid, resolved.info.GetUnlockConditionId(),
	)
	if err != nil {
		return nil, err
	}
	if !unlocked {
		return nil, viber.FailedPreconditionf("panorama shooting is locked")
	}
	visible, err := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli()).Satisfied(
		resolved.info.GetViewConditionId(),
	)
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, viber.FailedPreconditionf("panorama music %q is not available", req.GetPhotoMusicId())
	}
	if err := validateOwnedPhotoStyles(
		ctx, suid, req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds(),
	); err != nil {
		return nil, err
	}
	return resolved, nil
}

func listPhotoPanoramaMusics(ctx context.Context) ([]*resolvedPhotoPanoramaMusic, error) {
	catalog, err := viber.LoadPhotoPanoramaCatalog()
	if err != nil {
		return nil, err
	}
	overrides := photoPanoramaCatalogEntriesByMusicID(catalog)

	musics, err := opmaster.FindAllMusics(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(musics, func(i, j int) bool { return musics[i].GetId() < musics[j].GetId() })
	result := make([]*resolvedPhotoPanoramaMusic, 0, len(catalog.Musics))
	for _, music := range musics {
		if !music.GetIs360LiveExist() || music.GetId() == "" {
			continue
		}
		entry, configured := overrides[music.GetId()]
		if !configured {
			continue
		}
		resolved, err := resolvePhotoPanoramaMusicWithCatalog(ctx, music, entry)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) || status.Code(err) == codes.FailedPrecondition {
				mcontext.GetLogger(ctx).Warnw(
					"omitting unavailable panorama music",
					"music_id", music.GetId(),
					"error", err,
				)
				continue
			}
			return nil, err
		}
		result = append(result, resolved)
	}
	return result, nil
}

func resolvePhotoPanoramaMusic(ctx context.Context, musicID string) (*resolvedPhotoPanoramaMusic, error) {
	music, err := opmaster.FindMusic(ctx, musicID)
	if err != nil {
		return nil, photoMasterLookupError("panorama music", musicID, err)
	}
	if !music.GetIs360LiveExist() {
		return nil, viber.InvalidArgumentf("music %q does not support panorama shooting", musicID)
	}
	catalog, err := viber.LoadPhotoPanoramaCatalog()
	if err != nil {
		return nil, err
	}
	entry, configured := photoPanoramaCatalogEntriesByMusicID(catalog)[musicID]
	if !configured {
		return nil, viber.FailedPreconditionf("panorama music %q is not configured", musicID)
	}
	return resolvePhotoPanoramaMusicWithCatalog(ctx, music, entry)
}

func resolvePhotoPanoramaMusicWithCatalog(
	ctx context.Context,
	music *pmaster.Music,
	entry viber.PhotoPanoramaCatalogEntry,
) (*resolvedPhotoPanoramaMusic, error) {
	stage, err := opmaster.FindPhotoStage(ctx, entry.StageID)
	if err != nil {
		return nil, err
	}
	if len(stage.GetCameraPositions()) == 0 {
		return nil, viber.FailedPreconditionf("panorama music %q has no stage with camera positions", music.GetId())
	}
	return &resolvedPhotoPanoramaMusic{
		info: &papi.PhotoPanoramaMusic{
			MusicId:           music.GetId(),
			StageId:           stage.GetId(),
			UnlockConditionId: entry.UnlockConditionID,
			AssetId:           music.GetAssetId(),
			Order:             entry.Order,
			ViewConditionId:   entry.ViewConditionID,
		},
		music: music,
		stage: stage,
	}, nil
}

func photoPanoramaCatalogEntriesByMusicID(
	catalog *viber.PhotoPanoramaCatalog,
) map[string]viber.PhotoPanoramaCatalogEntry {
	entries := make(map[string]viber.PhotoPanoramaCatalogEntry, len(catalog.Musics))
	for _, entry := range catalog.Musics {
		entries[entry.MusicID] = entry
	}
	return entries
}

func photoPanoramaUnlockConditionSatisfied(ctx context.Context, suid, conditionID string) (bool, error) {
	if conditionID != photoPanoramaShootingTutorialConditionID {
		return viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli()).Satisfied(conditionID)
	}
	tutorials, err := opuser.FindAllUserTutorials(ctx, suid)
	if err != nil {
		return false, err
	}
	for _, tutorial := range tutorials {
		if tutorial.GetTutorialType() == penum.TutorialType_TutorialType_PhotoCreateShooting &&
			tutorial.GetStep() >= 1 {
			return true, nil
		}
	}
	return false, nil
}

func validPhotoPanoramaCameraType(value penum.PanoramaCameraType) bool {
	switch value {
	case penum.PanoramaCameraType_PanoramaCameraType_StageCamera,
		penum.PanoramaCameraType_PanoramaCameraType_DollyCamera,
		penum.PanoramaCameraType_PanoramaCameraType_OshiCamera,
		penum.PanoramaCameraType_PanoramaCameraType_DroneCamera,
		penum.PanoramaCameraType_PanoramaCameraType_AutoCamera:
		return true
	default:
		return false
	}
}

func photoPanoramaCameraPositionExists(stage *pmaster.Stage, index int32) bool {
	for _, position := range stage.GetCameraPositions() {
		if position.GetIndex() == index {
			return true
		}
	}
	return false
}

func hasDuplicatePhotoPanoramaCharacters(characterIDs []string) bool {
	seen := make(map[string]struct{}, len(characterIDs))
	for _, characterID := range characterIDs {
		if _, duplicate := seen[characterID]; duplicate {
			return true
		}
		seen[characterID] = struct{}{}
	}
	return false
}
