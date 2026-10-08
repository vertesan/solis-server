package live

import (
	"embed"
	"fmt"
	"sync"

	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.yaml.in/yaml/v3"
)

//go:embed scoring.yaml
var scoringFiles embed.FS

var loadBeatCoefficientOverrides = sync.OnceValues(func() (map[string]int32, error) {
	data, err := scoringFiles.ReadFile("scoring.yaml")
	if err != nil {
		return nil, err
	}
	var rules struct {
		Overrides map[string]int32 `yaml:"beatScoreCoefficientPermilOverrides"`
	}
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return nil, err
	}
	for id, coefficient := range rules.Overrides {
		if id == "" || coefficient <= 0 || coefficient > 4000 {
			return nil, fmt.Errorf("invalid beat score coefficient for %q", id)
		}
	}
	return rules.Overrides, nil
})

// BeatScoreCoefficientPermil approximates the song's normalization using its
// reference (-001) chart, regardless of which quest chart is being played.
// Captures support rounding 8000/(reference beats+1) to an integer permil;
// neither the official reference selection nor the +1 convention is proven.
// Missing/skill-only reference charts fall back to the actual chart, using the
// same formula. A live with no ordinary beats needs no beat coefficient.
func BeatScoreCoefficientPermil(musicID string, referenceChart, actualChart []*pmaster.MusicChartPattern) (int32, error) {
	count := func(chart []*pmaster.MusicChartPattern) (int64, error) {
		var beats int64
		for _, row := range chart {
			if row == nil {
				return 0, fmt.Errorf("nil live chart row")
			}
			if row.GetType() == penum.MusicChartType_MusicChartType_Beat {
				beats++
			}
		}
		return beats, nil
	}
	beats, err := count(referenceChart)
	if err != nil {
		return 0, err
	}
	if beats == 0 {
		beats, err = count(actualChart)
		if err != nil {
			return 0, err
		}
	}
	if beats == 0 {
		return 0, nil
	}
	overrides, err := loadBeatCoefficientOverrides()
	if err != nil {
		return 0, err
	}
	if value := overrides[musicID]; value != 0 {
		return value, nil
	}
	divisor := beats + 1
	return int32((8000 + divisor/2) / divisor), nil
}
