package smodel

import "fmt"

// GachaCatalog selects the ongoing rotation without altering official master
// data. Explicit rosters may reopen past events while retaining prerequisites.
type GachaCatalog struct {
	UseMasterCatalog    bool     `yaml:"useMasterCatalog" json:"useMasterCatalog" bson:"useMasterCatalog"`
	GachaIds            []string `yaml:"gachaIds" json:"gachaIds" bson:"gachaIds"`
	IgnoreOfficialDates bool     `yaml:"ignoreOfficialDates" json:"ignoreOfficialDates" bson:"ignoreOfficialDates"`
}

func (catalog *GachaCatalog) Validate() error {
	if catalog == nil {
		return fmt.Errorf("gacha catalog is required")
	}
	seen := make(map[string]struct{}, len(catalog.GachaIds))
	for _, id := range catalog.GachaIds {
		if id == "" {
			return fmt.Errorf("gacha IDs cannot be empty")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("gacha catalog repeats %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}
