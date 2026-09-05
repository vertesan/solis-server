package viber

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	PhotoEnhanceItemID             = "item-photo_enhance"
	PhotoRerollItemID              = "item-photo_reroll"
	photoDedicatedAbilityMissionID = "ms-photo-manager_level_achievement-80"
	photoDedicatedAbilityMinLevel  = 80
)

// Photo generation tuning. Each rarity row starts at its minimum rarity and
// must total 100. For example, minimum rarity 3 uses ★3/★4/★5 percentages.
var photoRarityPercentagesByMinimumRarity = map[int32][]int{
	1: {55, 25, 12, 6, 2},
	2: {60, 25, 10, 5},
	3: {70, 25, 5},
	4: {90, 10},
	5: {100},
}

// OrdinaryDedicatedPhotoChancePercent applies after an ordinary film produces
// a ★5 photo. Set it to zero to disable ordinary dedicated-photo drops.
var OrdinaryDedicatedPhotoChancePercent = 1

// UnlockManagerLevelPhotoAbilities makes mission-bound dedicated abilities
// usable as soon as their manager-level gate has been reached.
func UnlockManagerLevelPhotoAbilities(
	ctx context.Context,
	suid string,
	managerLevel int32,
) ([]*ptransaction.UserPhoto, error) {
	if managerLevel < photoDedicatedAbilityMinLevel {
		return []*ptransaction.UserPhoto{}, nil
	}
	photos, err := opuser.FindAllUserPhotos(ctx, suid)
	if err != nil {
		return nil, err
	}
	changed := make([]*ptransaction.UserPhoto, 0)
	for _, photo := range photos {
		photoChanged := false
		for _, ability := range photo.GetAbilities() {
			if ability.GetMissionId() == photoDedicatedAbilityMissionID && !ability.GetIsAvailable() {
				ability.IsAvailable = true
				photoChanged = true
			}
		}
		if !photoChanged {
			continue
		}
		if err := opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
			return nil, err
		}
		changed = append(changed, photo)
	}
	return changed, nil
}

// PhotoGenerator owns the master-data snapshot used for one shooting request.
// It is intentionally request-scoped: master data can be updated without a
// process restart, and the decoded documents need not live for the lifetime of
// the server.
type PhotoGenerator struct {
	abilitySets           []*pmaster.PhotoAbilitySet
	DedicatedCharacterIDs []string
	qualityAnchors        map[int32]int32
	thresholds            []int32
	minLevel              int32
	maxLevel              int32
}

type GeneratedPhotoTraits struct {
	Rarity      int32
	Level       int32
	Abilities   []*ptransaction.UserPhotoAbility
	Dedicated   bool
	CharacterID string
}

func NewPhotoGenerator(ctx context.Context, setting *pmaster.Setting) (*PhotoGenerator, error) {
	sets, err := opmaster.FindAllPhotoAbilitySets(ctx)
	if err != nil {
		return nil, err
	}
	characters, err := opmaster.FindAllCharacters(ctx)
	if err != nil {
		return nil, err
	}
	items, err := opmaster.FindAllItems(ctx)
	if err != nil {
		return nil, err
	}
	levels, err := opmaster.FindAllPhotoRarityLevels(ctx)
	if err != nil {
		return nil, err
	}
	if len(levels) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "photo rarity-level master data is empty")
	}
	minLevel, maxLevel := levels[0].GetLevel(), levels[0].GetLevel()
	for _, level := range levels[1:] {
		minLevel = min(minLevel, level.GetLevel())
		maxLevel = max(maxLevel, level.GetLevel())
	}
	anchors := make(map[int32]int32)
	for _, item := range items {
		if item.GetType() != penum.ItemType_ItemType_PhotoShooting {
			continue
		}
		quality, ok := photoQualityFromDescription(item.GetDescription())
		if !ok {
			continue
		}
		if current := anchors[item.GetValue()]; quality > current {
			anchors[item.GetValue()] = quality
		}
	}
	thresholds := slices.Clone(setting.GetPhotoShootingItemThresholds())
	slices.Sort(thresholds)
	generator := &PhotoGenerator{
		abilitySets:    sets,
		qualityAnchors: anchors,
		thresholds:     thresholds,
		minLevel:       minLevel,
		maxLevel:       maxLevel,
	}
	for _, character := range characters {
		if len(generator.dedicatedSets(character.GetId())) > 0 {
			generator.DedicatedCharacterIDs = append(generator.DedicatedCharacterIDs, character.GetId())
		}
	}
	return generator, nil
}

func (g *PhotoGenerator) Generate(
	ctx context.Context,
	item *pmaster.Item,
	focusCharacterID string,
	managerLevel int32,
) (*GeneratedPhotoTraits, error) {
	if item == nil {
		return nil, status.Error(codes.FailedPrecondition, "shooting film master data is required")
	}
	if item.GetType() != penum.ItemType_ItemType_PhotoShooting &&
		item.GetType() != penum.ItemType_ItemType_PhotoShootingLevelChange {
		return nil, status.Errorf(codes.FailedPrecondition, "item %q is not a shooting film", item.GetId())
	}

	guaranteedFive := strings.Contains(item.GetName(), "★5確定") || item.GetIsPhotoShootingCharacterOnly()
	rarity := int32(5)
	if !guaranteedFive {
		rarity = g.randomRarity(item.GetValue())
	}
	dedicated := item.GetIsPhotoShootingCharacterOnly()
	if !dedicated && rarity == 5 && rand.IntN(100) < OrdinaryDedicatedPhotoChancePercent {
		dedicated = g.hasDedicatedSet(focusCharacterID)
	}

	level := g.randomLevel(item, managerLevel, guaranteedFive)
	abilities, usedDedicated, err := g.RandomAbilities(ctx, rarity, level, focusCharacterID, dedicated)
	if err != nil {
		return nil, err
	}
	ConfigureDedicatedPhotoAbility(abilities, usedDedicated, managerLevel)
	return &GeneratedPhotoTraits{
		Rarity:      rarity,
		Level:       level,
		Abilities:   abilities,
		Dedicated:   usedDedicated,
		CharacterID: focusCharacterID,
	}, nil
}

func ConfigureDedicatedPhotoAbility(
	abilities []*ptransaction.UserPhotoAbility,
	dedicated bool,
	managerLevel int32,
) {
	if !dedicated || len(abilities) == 0 ||
		!strings.HasPrefix(abilities[0].GetPhotoAbilityId(), "pab-passive-skill_") {
		return
	}
	// Captured dedicated photos consistently bind their passive first slot to
	// the manager-level-80 mission. Dedicated non-passive abilities have no
	// mission binding.
	abilities[0].MissionId = photoDedicatedAbilityMissionID
	abilities[0].IsAvailable = managerLevel >= photoDedicatedAbilityMinLevel
}

func (g *PhotoGenerator) RandomAbilities(
	ctx context.Context,
	rarity int32,
	level int32,
	focusCharacterID string,
	dedicated bool,
) ([]*ptransaction.UserPhotoAbility, bool, error) {
	if dedicated {
		sets := g.dedicatedSets(focusCharacterID)
		if len(sets) == 0 {
			return nil, false, status.Errorf(
				codes.FailedPrecondition,
				"character %q has no dedicated photo ability set",
				focusCharacterID,
			)
		}
		abilities, err := g.abilitiesFromCandidates(ctx, sets, level)
		return abilities, true, err
	}
	abilities, err := g.abilitiesFromCandidates(ctx, g.genericSets(rarity), level)
	return abilities, false, err
}

func (g *PhotoGenerator) abilitiesFromCandidates(
	ctx context.Context,
	candidates []*pmaster.PhotoAbilitySet,
	level int32,
) ([]*ptransaction.UserPhotoAbility, error) {
	if len(candidates) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "no compatible photo ability set exists")
	}
	return BuildPhotoAbilitiesFromSet(ctx, candidates[rand.IntN(len(candidates))], level)
}

func (g *PhotoGenerator) genericSets(rarity int32) []*pmaster.PhotoAbilitySet {
	prefix := fmt.Sprintf("pab-set-%d-", rarity)
	result := make([]*pmaster.PhotoAbilitySet, 0)
	for _, set := range g.abilitySets {
		if !set.GetLimited() && strings.HasPrefix(set.GetId(), prefix) && !strings.Contains(set.GetId(), "recipe") {
			result = append(result, set)
		}
	}
	return result
}

func (g *PhotoGenerator) dedicatedSets(characterID string) []*pmaster.PhotoAbilitySet {
	shortID := strings.TrimPrefix(characterID, "char-")
	needle := "-" + shortID + "-idol-"
	result := make([]*pmaster.PhotoAbilitySet, 0)
	for _, set := range g.abilitySets {
		if set.GetPhotoRarity() == 5 && set.GetLimited() && strings.Contains(set.GetId(), needle) {
			result = append(result, set)
		}
	}
	return result
}

func (g *PhotoGenerator) hasDedicatedSet(characterID string) bool {
	return len(g.dedicatedSets(characterID)) > 0
}

func (g *PhotoGenerator) randomRarity(filmLevel int32) int32 {
	minimum := int32(1)
	for index, threshold := range g.thresholds {
		if filmLevel >= threshold {
			minimum = int32(index + 1)
		}
	}
	minimum = min(max(minimum, 1), 5)
	weights := photoRarityPercentagesByMinimumRarity[minimum]
	draw := rand.IntN(100)
	for index, weight := range weights {
		if draw < weight {
			return minimum + int32(index)
		}
		draw -= weight
	}
	return 5
}

func (g *PhotoGenerator) randomLevel(item *pmaster.Item, managerLevel int32, fixed bool) int32 {
	base := g.qualityForFilmLevel(item.GetValue())
	if quality, ok := photoQualityFromDescription(item.GetDescription()); ok {
		base = quality
	}
	if item.GetType() == penum.ItemType_ItemType_PhotoShootingLevelChange {
		if quality, ok := photoMaximumQualityFromDescription(item.GetDescription()); ok {
			base = min(managerLevel, quality)
		} else {
			base = managerLevel
		}
	}
	if !fixed {
		base += []int32{-10, -5, 0, 5, 10, 15}[rand.IntN(6)]
	}
	base = ((base + 2) / 5) * 5
	return min(max(base, g.minLevel), g.maxLevel)
}

func (g *PhotoGenerator) qualityForFilmLevel(filmLevel int32) int32 {
	if value, ok := g.qualityAnchors[filmLevel]; ok {
		return value
	}
	lowerLevel, lowerQuality := int32(-1), int32(0)
	upperLevel, upperQuality := int32(1<<30), int32(0)
	for level, quality := range g.qualityAnchors {
		if level < filmLevel && level > lowerLevel {
			lowerLevel, lowerQuality = level, quality
		}
		if level > filmLevel && level < upperLevel {
			upperLevel, upperQuality = level, quality
		}
	}
	if lowerLevel >= 0 && upperLevel < 1<<30 {
		return lowerQuality + (upperQuality-lowerQuality)*(filmLevel-lowerLevel)/(upperLevel-lowerLevel)
	}
	if lowerLevel >= 0 {
		return lowerQuality + 10*(filmLevel-lowerLevel)
	}
	if upperLevel < 1<<30 {
		return max(g.minLevel, upperQuality-20*(upperLevel-filmLevel))
	}
	return max(g.minLevel, filmLevel*20)
}

func photoQualityFromDescription(description string) (int32, bool) {
	return photoNumberAfter(description, "品質")
}

func photoMaximumQualityFromDescription(description string) (int32, bool) {
	return photoNumberAfter(description, "最大品質:")
}

func photoNumberAfter(value, marker string) (int32, bool) {
	index := strings.Index(value, marker)
	if index < 0 {
		return 0, false
	}
	tail := value[index+len(marker):]
	end := 0
	for end < len(tail) && tail[end] >= '0' && tail[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	parsed, err := strconv.ParseInt(tail[:end], 10, 32)
	return int32(parsed), err == nil
}

func BuildPhotoAbilitiesFromSet(
	ctx context.Context,
	set *pmaster.PhotoAbilitySet,
	level int32,
) ([]*ptransaction.UserPhotoAbility, error) {
	if set == nil || len(set.GetPhotoAbilityIds()) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "photo ability set is empty")
	}
	masters, err := opmaster.FindPhotoAbilities(ctx, set.GetPhotoAbilityIds())
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*pmaster.PhotoAbility, len(masters))
	for _, master := range masters {
		byID[master.GetId()] = master
	}
	result := make([]*ptransaction.UserPhotoAbility, 0, len(set.GetPhotoAbilityIds()))
	for _, abilityID := range set.GetPhotoAbilityIds() {
		master := byID[abilityID]
		if master == nil {
			return nil, status.Errorf(codes.FailedPrecondition, "photo ability %q is absent", abilityID)
		}
		grades := master.GetPhotoAbilityGrades()
		if len(grades) == 0 {
			return nil, status.Errorf(codes.FailedPrecondition, "photo ability %q has no grades", abilityID)
		}
		grade := grades[rand.IntN(len(grades))].GetGrade()
		effect, err := PhotoAbilityEffectValue(master, level, grade)
		if err != nil {
			return nil, err
		}
		result = append(result, &ptransaction.UserPhotoAbility{
			PhotoAbilityId: abilityID,
			EffectValue:    effect,
			Grade:          grade,
			IsAvailable:    true,
		})
	}
	return result, nil
}

func PhotoAbilityEffectValue(master *pmaster.PhotoAbility, level, grade int32) (int64, error) {
	var selectedLevel *pmaster.PhotoAbilityLevel
	for _, candidate := range master.GetPhotoAbilityLevels() {
		if candidate.GetLevel() <= level && (selectedLevel == nil || candidate.GetLevel() > selectedLevel.GetLevel()) {
			selectedLevel = candidate
		}
	}
	if selectedLevel == nil {
		for _, candidate := range master.GetPhotoAbilityLevels() {
			if selectedLevel == nil || candidate.GetLevel() < selectedLevel.GetLevel() {
				selectedLevel = candidate
			}
		}
	}
	var selectedGrade *pmaster.PhotoAbilityGrade
	for _, candidate := range master.GetPhotoAbilityGrades() {
		if candidate.GetGrade() == grade {
			selectedGrade = candidate
			break
		}
	}
	if selectedLevel == nil || selectedGrade == nil {
		return 0, status.Errorf(codes.FailedPrecondition, "photo ability %q lacks level %d or grade %d", master.GetId(), level, grade)
	}
	return int64(selectedLevel.GetValue())*int64(selectedGrade.GetBonusPermil())/1000 + int64(selectedGrade.GetBonusValue()), nil
}

func AppendUniquePhotoMissions(target []*ptransaction.UserMission, values ...[]*ptransaction.UserMission) []*ptransaction.UserMission {
	seen := make(map[string]struct{}, len(target))
	for _, mission := range target {
		seen[mission.GetMissionId()] = struct{}{}
	}
	for _, group := range values {
		for _, mission := range group {
			if mission == nil {
				continue
			}
			if _, exists := seen[mission.GetMissionId()]; exists {
				continue
			}
			seen[mission.GetMissionId()] = struct{}{}
			target = append(target, mission)
		}
	}
	return target
}
