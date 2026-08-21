package viber

import (
	"embed"
	"fmt"

	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.yaml.in/yaml/v3"
)

// diaryCatalogFiles keeps only the embedded file index alive. ReadFile's YAML
// allocation is local to LoadDiaryCatalog and can be reclaimed after decoding.
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

func LoadDiaryCatalog() ([]*papi.DiaryInfo, error) {
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
