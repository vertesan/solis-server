package dream

import (
	"context"
	"math"
	"strings"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Generation is intentionally an approximation: the server-only official
// formulas are absent from master data. Captures establish the five stage-count
// bands, 2-5 manuals, quality/type/option-dependent plans, and bounded options.
// Keep these balancing choices together rather than baking in captured photos.
const (
	minimumGrowthPermil = 100
	maximumGrowthPermil = 600
)

func BaseEligible(photo *ptransaction.UserPhoto) bool {
	return photo != nil && photo.GetRarity() >= 4 && photo.GetLevel() > 0 &&
		!strings.HasPrefix(photo.GetAssetId(), "phot-clb-2304-") && len(photo.GetAbilities()) > 0
}

func MaterialEligible(base, material *ptransaction.UserPhoto, favoriteID string) bool {
	return base != nil && material != nil && base.GetPhotoId() != material.GetPhotoId() &&
		material.GetRarity() >= 4 && material.GetLevel() > 0 && !material.GetLocked() &&
		!material.GetIsDreamBase() && material.GetPhotoId() != favoriteID &&
		(base.GetPhotoRecipeId() == "") == (material.GetPhotoRecipeId() == "")
}

func LoadAbilities(ctx context.Context, photos []*ptransaction.UserPhoto) (map[string]*pmaster.PhotoAbility, error) {
	ids := make([]string, 0)
	seen := make(map[string]bool)
	for _, photo := range photos {
		for _, ability := range photo.GetAbilities() {
			if id := ability.GetPhotoAbilityId(); id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	rows, err := opmaster.FindPhotoAbilities(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*pmaster.PhotoAbility, len(rows))
	for _, row := range rows {
		result[row.GetId()] = row
	}
	return result, nil
}

// Plan is deterministic, so a material preview and the committed generation
// agree exactly. Effect caps come from each ability's actual master levels and
// grades, including discrete photo-skill levels.
func Plan(base, material *ptransaction.UserPhoto, masters map[string]*pmaster.PhotoAbility) (*papi.DreamUserMaterialPhoto, error) {
	if !BaseEligible(base) || material == nil || len(material.GetAbilities()) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "photos cannot generate an image-training area")
	}
	materialTypes := make(map[penum.PhotoAbilityType]bool)
	for _, a := range material.GetAbilities() {
		if m := masters[a.GetPhotoAbilityId()]; m != nil {
			materialTypes[m.GetAbilityType()] = true
		}
	}
	var matching int32
	var dedicated bool
	for _, a := range base.GetAbilities() {
		m := masters[a.GetPhotoAbilityId()]
		if m == nil {
			return nil, status.Errorf(codes.FailedPrecondition, "unknown photo ability %q", a.GetPhotoAbilityId())
		}
		if materialTypes[m.GetAbilityType()] {
			matching++
		}
		if m.GetAbilityType() == penum.PhotoAbilityType_PhotoAbilityType_PassiveSkill && len(base.GetCharacterIds()) == 1 {
			dedicated = true
		}
	}
	similarity := matching * 100 / int32(len(base.GetAbilities()))
	quality := min(int32(1200), material.GetLevel()*1000/max(int32(1), base.GetLevel()))
	var stages int32
	switch {
	case quality < 400:
		stages = 15
	case quality < 600:
		stages = 20
	case quality < 800:
		stages = 25
	case quality < 1000:
		stages = 35
	default:
		stages = 50
	}
	if material.GetRarity() < base.GetRarity() && stages > 15 {
		stages = max(int32(15), stages-15)
		if stages == 10 {
			stages = 15
		}
	}
	// Round down to an observed band after the rarity penalty.
	if stages > 25 && stages < 35 {
		stages = 25
	}
	growth := min(int32(maximumGrowthPermil), max(int32(minimumGrowthPermil), quality/5+similarity*2))
	points := int64(base.GetLevel())*45 + int64(base.GetRarity())*250
	if dedicated {
		points += 500
	}
	if base.GetPhotoRecipeId() != "" {
		points += 300
	}
	if base.GetImageType() == penum.PhotoImageType_PhotoImageType_Asset {
		points += 200
	}
	points += int64(max(int32(0), material.GetLevel()-base.GetLevel())) * 5
	rank, plus := rankForPoints(points)
	result := &papi.DreamUserMaterialPhoto{PhotoId: material.GetPhotoId(), AreaRankType: rank, AreaRankPlus: plus,
		MaxQuestNumber: stages, RankPoint: points, GenerateAreaItemConsumptionAmount: 2 + min(int32(3), (100-similarity)/35+max(int32(0), base.GetLevel()-material.GetLevel())/80),
		AttributeInfo: attributeInfo(base, masters)}
	var originalSum, enhancedSum int64
	for _, a := range base.GetAbilities() {
		m := masters[a.GetPhotoAbilityId()]
		cap := abilityMaximum(m)
		before := max(int64(0), a.GetEffectValue())
		bonus := before * int64(growth) / 1000
		if m.GetAbilityType() == penum.PhotoAbilityType_PhotoAbilityType_PassiveSkill {
			bonus = max(int64(1), bonus)
		}
		// Availability is governed by the original mission, not by Dream clears.
		if !a.GetIsAvailable() {
			bonus = 0
		}
		after := max(before, min(cap, before+bonus))
		result.EnhancePlanAbilities = append(result.EnhancePlanAbilities, &papi.DreamUserPhotoAbility{PhotoAbilityId: a.GetPhotoAbilityId(), MaxEffectValue: after, MaxDisplayValue: max(cap, after)})
		originalSum += before
		enhancedSum += after
	}
	result.MaxGrowthPermil = 1000
	if originalSum > 0 {
		result.MaxGrowthPermil = int32(enhancedSum * 1000 / originalSum)
	}
	return result, nil
}

func rankForPoints(points int64) (penum.DreamAreaRankType, int64) {
	switch {
	case points < 3000:
		return penum.DreamAreaRankType_DreamAreaRankType_D, 0
	case points < 4500:
		return penum.DreamAreaRankType_DreamAreaRankType_C, 0
	case points < 7000:
		return penum.DreamAreaRankType_DreamAreaRankType_B, 0
	case points < 8000:
		return penum.DreamAreaRankType_DreamAreaRankType_A, 0
	case points < 9000:
		return penum.DreamAreaRankType_DreamAreaRankType_S, 0
	default:
		return penum.DreamAreaRankType_DreamAreaRankType_Splus, min(int64(10), 1+(points-9000)/400)
	}
}

func abilityMaximum(master *pmaster.PhotoAbility) int64 {
	var value int64
	for _, level := range master.GetPhotoAbilityLevels() {
		for _, grade := range master.GetPhotoAbilityGrades() {
			value = max(value, int64(level.GetValue())*int64(grade.GetBonusPermil())/1000+int64(grade.GetBonusValue()))
		}
	}
	return value
}

func attributeInfo(photo *ptransaction.UserPhoto, masters map[string]*pmaster.PhotoAbility) *papi.DreamAreaAttributeInfo {
	var vocal, dance, visual int64
	for _, a := range photo.GetAbilities() {
		switch masters[a.GetPhotoAbilityId()].GetAbilityType() {
		case penum.PhotoAbilityType_PhotoAbilityType_VocalAdd, penum.PhotoAbilityType_PhotoAbilityType_VocalMultiply:
			vocal += a.GetEffectValue()
		case penum.PhotoAbilityType_PhotoAbilityType_DanceAdd, penum.PhotoAbilityType_PhotoAbilityType_DanceMultiply:
			dance += a.GetEffectValue()
		case penum.PhotoAbilityType_PhotoAbilityType_VisualAdd, penum.PhotoAbilityType_PhotoAbilityType_VisualMultiply:
			visual += a.GetEffectValue()
		}
	}
	info := &papi.DreamAreaAttributeInfo{ActiveSkillWeightPermil: 1000, SpecialSkillWeightPermil: 1000, SkillStaminaWeightPermil: 1000, StaminaRecoveryWeightPermil: 1000,
		BeatVocalWeightPermil: 200, BeatDanceWeightPermil: 200, BeatVisualWeightPermil: 200, QuestPressureId: "quest_pressure-2", AttributeType: penum.AttributeType_AttributeType_Vocal}
	switch {
	case dance > vocal && dance >= visual:
		info.AttributeType = penum.AttributeType_AttributeType_Dance
		info.BeatDanceWeightPermil = 600
	case visual > vocal && visual > dance:
		info.AttributeType = penum.AttributeType_AttributeType_Visual
		info.BeatVisualWeightPermil = 600
	default:
		info.BeatVocalWeightPermil = 600
	}
	return info
}

// Growth is slightly back-loaded, like the captured final-stage increases.
func grownValue(original, maximum int64, cleared, total int32) int64 {
	if cleared <= 0 || total <= 0 {
		return original
	}
	if cleared >= total {
		return maximum
	}
	fraction := float64(cleared) / float64(total)
	return original + int64(math.Floor(float64(maximum-original)*fraction*fraction))
}
