package photopanorama

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.yaml.in/yaml/v3"
)

// photoPanoramaCatalogFiles retains only the embedded file index. The raw
// YAML bytes read by loadPhotoPanoramaCatalog are local and can be reclaimed
// after decoding.
//
//go:embed catalogdata/photo_panorama.yaml
var photoPanoramaCatalogFiles embed.FS

type PhotoPanoramaCatalog struct {
	Musics []PhotoPanoramaCatalogEntry `yaml:"musics"`
}

type PhotoPanoramaCatalogEntry struct {
	MusicID           string `yaml:"musicId"`
	StageID           string `yaml:"stageId"`
	Order             int32  `yaml:"order"`
	UnlockConditionID string `yaml:"unlockConditionId"`
	ViewConditionID   string `yaml:"viewConditionId"`
}

func loadPhotoPanoramaCatalog() (*PhotoPanoramaCatalog, error) {
	data, err := photoPanoramaCatalogFiles.ReadFile("catalogdata/photo_panorama.yaml")
	if err != nil {
		return nil, fmt.Errorf("read photo panorama catalog: %w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var catalog PhotoPanoramaCatalog
	if err := decoder.Decode(&catalog); err != nil {
		return nil, fmt.Errorf("decode photo panorama catalog: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode photo panorama catalog: multiple YAML documents are not supported")
		}
		return nil, fmt.Errorf("decode photo panorama catalog: %w", err)
	}

	if len(catalog.Musics) == 0 {
		return nil, fmt.Errorf("photo panorama catalog is empty")
	}
	musicIDs := make(map[string]struct{}, len(catalog.Musics))
	orders := make(map[int32]struct{}, len(catalog.Musics))
	for index, entry := range catalog.Musics {
		if entry.MusicID == "" || entry.StageID == "" || entry.UnlockConditionID == "" || entry.ViewConditionID == "" {
			return nil, fmt.Errorf("photo panorama catalog entry %d has an empty required field", index)
		}
		if entry.MusicID != strings.TrimSpace(entry.MusicID) ||
			entry.StageID != strings.TrimSpace(entry.StageID) ||
			entry.UnlockConditionID != strings.TrimSpace(entry.UnlockConditionID) ||
			entry.ViewConditionID != strings.TrimSpace(entry.ViewConditionID) {
			return nil, fmt.Errorf("photo panorama catalog entry %d has surrounding whitespace", index)
		}
		if entry.Order <= 0 {
			return nil, fmt.Errorf("photo panorama catalog entry %q has a non-positive order", entry.MusicID)
		}
		if _, duplicate := musicIDs[entry.MusicID]; duplicate {
			return nil, fmt.Errorf("photo panorama catalog has duplicate musicId %q", entry.MusicID)
		}
		if _, duplicate := orders[entry.Order]; duplicate {
			return nil, fmt.Errorf("photo panorama catalog has duplicate order %d", entry.Order)
		}
		musicIDs[entry.MusicID] = struct{}{}
		orders[entry.Order] = struct{}{}
	}
	return &catalog, nil
}

const (
	photoPanoramaEventName                   = "360°LIVE "
	photoPanoramaShootingTutorialConditionID = "cd-tutorial-finish-photo-create-shooting"
)

type resolvedPhotoPanoramaMusic struct {
	info  *papi.PhotoPanoramaMusic
	music *pmaster.Music
	stage *pmaster.Stage
}

func checkPhotoPanoramaShooting(
	ctx context.Context,
	req *papi.PhotoPanoramaCheckShootingRequest,
) (*resolvedPhotoPanoramaMusic, error) {
	if req.GetPhotoMusicId() == "" {
		return nil, viber.InvalidArgumentf("photoMusicId is required")
	}
	if err := viber.ValidatePhotoStyleLists(req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds()); err != nil {
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
	if err := viber.ValidateOwnedPhotoStyles(
		ctx, suid, req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds(),
	); err != nil {
		return nil, err
	}
	return resolved, nil
}

func resolvePhotoPanoramaMusic(ctx context.Context, musicID string) (*resolvedPhotoPanoramaMusic, error) {
	music, err := opmaster.FindMusic(ctx, musicID)
	if err != nil {
		return nil, viber.PhotoMasterLookupError("panorama music", musicID, err)
	}
	if !music.GetIs360LiveExist() {
		return nil, viber.InvalidArgumentf("music %q does not support panorama shooting", musicID)
	}
	catalog, err := loadPhotoPanoramaCatalog()
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
	entry PhotoPanoramaCatalogEntry,
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
	catalog *PhotoPanoramaCatalog,
) map[string]PhotoPanoramaCatalogEntry {
	entries := make(map[string]PhotoPanoramaCatalogEntry, len(catalog.Musics))
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
