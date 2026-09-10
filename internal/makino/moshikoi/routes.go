package moshikoi

import (
	"embed"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"go.yaml.in/yaml/v3"
)

//go:embed routedata/*.yaml
var routeFiles embed.FS

// Route choices are one-based, matching the wire protocol (the handwritten
// route map numbers alternatives from zero). Each event has its own data file.
type Routes struct {
	LoveID           string                   `yaml:"loveId"`
	Episodes         map[string]*EpisodeRoute `yaml:"episodes"`
	Endings          []Ending                 `yaml:"endings"`
	EndingConditions map[string]string        `yaml:"endingConditions"`
}

type Ending struct {
	ID             string `yaml:"id"`
	Name           string `yaml:"name"`
	EndRollAssetID string `yaml:"endRollAssetId"`
	Favorability   int64  `yaml:"favorability"`
}

type EpisodeRoute struct {
	Groups   []RouteChoiceGroup `yaml:"groups"`
	Outcomes []RouteOutcome     `yaml:"outcomes"`
	Next     string             `yaml:"next"`
	Branches []RouteBranch      `yaml:"branches"`
}

type RouteChoiceGroup struct {
	Index    int32            `yaml:"index"`
	Count    int32            `yaml:"count"`
	Best     int32            `yaml:"best"`
	Scores   []int64          `yaml:"scores"`
	Requires []RouteCondition `yaml:"requires"`
}

type RouteCondition struct {
	Episode string `yaml:"episode"`
	Group   int32  `yaml:"group"`
	Choice  int32  `yaml:"choice"`
}

type RouteOutcome struct {
	All    []RouteCondition `yaml:"all"`
	Next   string           `yaml:"next"`
	Ending string           `yaml:"ending"`
}

type RouteBranch struct {
	Group     int32            `yaml:"group"`
	Match     int32            `yaml:"match"`
	Otherwise int32            `yaml:"otherwise"`
	All       []RouteCondition `yaml:"all"`
}

func FirstEpisodeID(loveID string) string { return "love-event-story-" + loveID + "-001" }

func LoadRoutes(loveID string) (*Routes, error) {
	if !slices.Contains(SupportedLoveIDs(), loveID) {
		return nil, fmt.Errorf("unsupported love route %s", loveID)
	}
	raw, err := routeFiles.ReadFile("routedata/" + loveID + ".yaml")
	if err != nil {
		return nil, err
	}
	var routes Routes
	if err := yaml.Unmarshal(raw, &routes); err != nil {
		return nil, err
	}
	if err := routes.validate(loveID); err != nil {
		return nil, err
	}
	return &routes, nil
}

func (r *Routes) validate(loveID string) error {
	if r.LoveID != loveID || r.Episodes[FirstEpisodeID(loveID)] == nil || len(r.Endings) == 0 {
		return fmt.Errorf("invalid route catalog for %s", loveID)
	}
	endings := make(map[string]bool, len(r.Endings))
	for _, ending := range r.Endings {
		if ending.ID == "" || ending.Name == "" || ending.Favorability < 0 || endings[ending.ID] {
			return fmt.Errorf("invalid love ending %s", ending.ID)
		}
		endings[ending.ID] = true
	}
	var scoreBound int64
	for id, episode := range r.Episodes {
		if episode == nil || !strings.HasPrefix(id, "love-event-story-"+loveID+"-") {
			return fmt.Errorf("invalid love route episode %s", id)
		}
		groups := make(map[int32]bool, len(episode.Groups))
		for _, group := range episode.Groups {
			if group.Index < 1 || groups[group.Index] || group.Count < 1 || group.Count > 100 || int(group.Count) != len(group.Scores) || group.Best < 0 || group.Best > group.Count {
				return fmt.Errorf("invalid choice group in %s", id)
			}
			groups[group.Index] = true
			for _, score := range group.Scores {
				if score == math.MinInt64 {
					return fmt.Errorf("love route score overflows")
				}
				if score < 0 {
					score = -score
				}
				if score > math.MaxInt64/4-scoreBound {
					return fmt.Errorf("love route score overflows")
				}
				scoreBound += score
			}
			if err := r.validateConditions(id, group.Requires); err != nil {
				return err
			}
		}
		if episode.Next != "" && r.Episodes[episode.Next] == nil || episode.Next == "" && len(episode.Outcomes) == 0 {
			return fmt.Errorf("invalid next love episode %s", id)
		}
		for _, outcome := range episode.Outcomes {
			if (outcome.Next == "") == (outcome.Ending == "") || outcome.Next != "" && r.Episodes[outcome.Next] == nil || outcome.Ending != "" && !endings[outcome.Ending] {
				return fmt.Errorf("invalid outcome in %s", id)
			}
			if err := r.validateConditions(id, outcome.All); err != nil {
				return err
			}
		}
		for _, branch := range episode.Branches {
			if branch.Group < 1 || branch.Match < 1 || branch.Otherwise < 1 {
				return fmt.Errorf("invalid story branch in %s", id)
			}
			if err := r.validateConditions(id, branch.All); err != nil {
				return err
			}
		}
	}
	for _, id := range r.EndingConditions {
		if !endings[id] {
			return fmt.Errorf("unknown ending condition target %s", id)
		}
	}
	return nil
}

func (r *Routes) validateConditions(currentEpisode string, conditions []RouteCondition) error {
	for _, condition := range conditions {
		id := condition.Episode
		if id == "" {
			id = currentEpisode
		}
		episode := r.Episodes[id]
		if episode == nil {
			return fmt.Errorf("unknown love condition episode %s", id)
		}
		index := slices.IndexFunc(episode.Groups, func(group RouteChoiceGroup) bool { return group.Index == condition.Group })
		if index < 0 || condition.Choice < 1 || condition.Choice > episode.Groups[index].Count {
			return fmt.Errorf("invalid love condition choice in %s", id)
		}
	}
	return nil
}

func (r *Routes) Episode(id string) *EpisodeRoute { return r.Episodes[id] }

func (r *Routes) Ending(id string) *Ending {
	for i := range r.Endings {
		if r.Endings[i].ID == id {
			return &r.Endings[i]
		}
	}
	return nil
}

func (r *Routes) AllEndingsUnlocked(ids []string) bool {
	for _, ending := range r.Endings {
		if !slices.Contains(ids, ending.ID) {
			return false
		}
	}
	return len(r.Endings) != 0
}

func routeConditionsMatch(currentEpisode string, conditions []RouteCondition, selections []umodel.LoveStorySelection) bool {
	for _, condition := range conditions {
		episode := condition.Episode
		if episode == "" {
			episode = currentEpisode
		}
		if !slices.ContainsFunc(selections, func(s umodel.LoveStorySelection) bool {
			return s.StoryEpisodeID == episode && s.GroupIndex == condition.Group && s.Index == condition.Choice
		}) {
			return false
		}
	}
	return true
}

// SelectChoices accepts the current valid alternative in each reachable group.
// The client also echoes previously seen alternatives with isValid=false;
// those are history, not current route choices, and cannot unlock a branch.
func (r *Routes) SelectChoices(episodeID string, choices []*papi.LoveSelectStoryChoiceInfo) ([]umodel.LoveStorySelection, error) {
	episode := r.Episode(episodeID)
	if episode == nil {
		return nil, fmt.Errorf("unknown love story episode")
	}
	selected := make([]umodel.LoveStorySelection, 0, len(episode.Groups))
	for _, choice := range choices {
		if choice == nil {
			return nil, fmt.Errorf("missing story choice")
		}
		groupIndex := slices.IndexFunc(episode.Groups, func(g RouteChoiceGroup) bool { return g.Index == choice.GroupIndex })
		if groupIndex < 0 || choice.Index < 1 || choice.Index > episode.Groups[groupIndex].Count {
			return nil, fmt.Errorf("invalid story choice")
		}
		if !choice.IsValid {
			continue
		}
		if slices.ContainsFunc(selected, func(s umodel.LoveStorySelection) bool { return s.GroupIndex == choice.GroupIndex }) {
			return nil, fmt.Errorf("multiple active choices for one group")
		}
		selected = append(selected, umodel.LoveStorySelection{StoryEpisodeID: episodeID, GroupIndex: choice.GroupIndex, Index: choice.Index})
	}
	for _, group := range episode.Groups {
		reachable := routeConditionsMatch(episodeID, group.Requires, selected)
		present := slices.ContainsFunc(selected, func(s umodel.LoveStorySelection) bool { return s.GroupIndex == group.Index })
		if reachable != present {
			return nil, fmt.Errorf("missing or unreachable story choice")
		}
	}
	return selected, nil
}

func (r *Routes) Resolve(episodeID string, selections []umodel.LoveStorySelection) (nextEpisodeID, endingID string, err error) {
	episode := r.Episode(episodeID)
	if episode == nil {
		return "", "", fmt.Errorf("unknown love story episode")
	}
	for _, outcome := range episode.Outcomes {
		if routeConditionsMatch(episodeID, outcome.All, selections) {
			return outcome.Next, outcome.Ending, nil
		}
	}
	if episode.Next == "" {
		return "", "", fmt.Errorf("love route has no matching outcome")
	}
	return episode.Next, "", nil
}

func (r *Routes) Favorability(selections []umodel.LoveStorySelection) int64 {
	var total int64
	for _, selection := range selections {
		episode := r.Episode(selection.StoryEpisodeID)
		if episode == nil {
			continue
		}
		for _, group := range episode.Groups {
			if group.Index == selection.GroupIndex && selection.Index > 0 && int(selection.Index) <= len(group.Scores) {
				total += group.Scores[selection.Index-1]
			}
		}
	}
	return total
}

func (r *Routes) ChoiceInfos(episodeID string, past []umodel.LoveStorySelection, hintLevel int32) []*papi.LoveStoryChoiceGroupInfo {
	var infos []*papi.LoveStoryChoiceGroupInfo
	for _, group := range r.Episode(episodeID).Groups {
		info := &papi.LoveStoryChoiceGroupInfo{GroupIndex: group.Index, ExistsObtainableFavorabilityDifference: hintLevel >= 1 && group.Best > 0}
		for index := int32(1); index <= group.Count; index++ {
			info.StoryChoiceInfos = append(info.StoryChoiceInfos, &papi.LoveStoryChoiceInfo{Index: index, IsBestObtainableFavorability: hintLevel >= 2 && group.Best == index, IsPastSelected: slices.Contains(past, umodel.LoveStorySelection{StoryEpisodeID: episodeID, GroupIndex: group.Index, Index: index})})
		}
		infos = append(infos, info)
	}
	return infos
}

func (r *Routes) BranchInfos(episodeID string, selections []umodel.LoveStorySelection) []*papi.LoveAdvBranchInfo {
	var infos []*papi.LoveAdvBranchInfo
	for _, branch := range r.Episode(episodeID).Branches {
		index := branch.Otherwise
		if routeConditionsMatch(episodeID, branch.All, selections) {
			index = branch.Match
		}
		infos = append(infos, &papi.LoveAdvBranchInfo{GroupIndex: branch.Group, Index: index})
	}
	return infos
}
