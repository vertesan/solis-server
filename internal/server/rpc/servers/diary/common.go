package diary

import (
	"context"
	"embed"
	"errors"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.yaml.in/yaml/v3"
)

// diaryCatalogFiles keeps only the embedded file index alive. ReadFile's YAML
// allocation is local to loadDiaryCatalog and can be reclaimed after decoding.
//
//go:embed catalogdata/diaries.yaml
var diaryCatalogFiles embed.FS

type diaryCatalogEntry struct {
	Id      string `yaml:"id"`
	Year    int32  `yaml:"year"`
	Month   int32  `yaml:"month"`
	Day     int32  `yaml:"day"`
	AssetId string `yaml:"assetId"`
}

func loadDiaryCatalog() ([]*papi.DiaryInfo, error) {
	data, err := diaryCatalogFiles.ReadFile("catalogdata/diaries.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read diary catalog: %w", err)
	}
	var entries []diaryCatalogEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("failed to decode diary catalog: %w", err)
	}
	infos := make([]*papi.DiaryInfo, 0, len(entries))
	for _, entry := range entries {
		infos = append(infos, &papi.DiaryInfo{
			Id: entry.Id, Year: entry.Year, Month: entry.Month, Day: entry.Day, AssetId: entry.AssetId,
		})
	}
	return infos, nil
}

func findDiaryStateOrEmpty(ctx context.Context, suid string) (*umodel.UserDiaryState, error) {
	state, err := opuser.FindUserDiaryState(ctx, suid)
	if err == nil {
		return state, nil
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return &umodel.UserDiaryState{SUID: suid, ReadIds: []string{}}, nil
	}
	return nil, err
}
