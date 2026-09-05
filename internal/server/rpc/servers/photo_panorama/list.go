package photopanorama

import (
	"context"
	"errors"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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

func listPhotoPanoramaMusics(ctx context.Context) ([]*resolvedPhotoPanoramaMusic, error) {
	catalog, err := loadPhotoPanoramaCatalog()
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
