package exercise

import (
	"embed"
	"fmt"
	"slices"
	"sync"

	"go.yaml.in/yaml/v3"
)

//go:embed scoring.yaml
var scoringFiles embed.FS

type scoringRules struct {
	Seed                         uint64           `yaml:"seed"`
	BeatScoreCoefficientPermil   map[string]int32 `yaml:"beatScoreCoefficientPermil"`
	DisableBaseCriticalExercises []string         `yaml:"disableBaseCriticalExercises"`
}

var loadScoringRules = sync.OnceValues(func() (*scoringRules, error) {
	data, err := scoringFiles.ReadFile("scoring.yaml")
	if err != nil {
		return nil, err
	}
	var rules scoringRules
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return nil, err
	}
	for id, value := range rules.BeatScoreCoefficientPermil {
		if id == "" || value <= 0 || value > 1000 {
			return nil, fmt.Errorf("invalid Exercise beat coefficient for %q", id)
		}
	}
	return &rules, nil
})

// ScoringParameters returns values rather than the cached map so callers
// cannot accidentally change later attempts' deterministic scoring rules.
func ScoringParameters(exerciseID, musicID string) (seed uint64, coefficientPermil int32, disableBaseCritical bool, err error) {
	rules, err := loadScoringRules()
	if err != nil {
		return 0, 0, false, err
	}
	return rules.Seed, rules.BeatScoreCoefficientPermil[musicID], slices.Contains(rules.DisableBaseCriticalExercises, exerciseID), nil
}
