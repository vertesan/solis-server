package dream

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func Generate(ctx context.Context, suid string, base, material *ptransaction.UserPhoto, plan *papi.DreamUserMaterialPhoto, now int64) (*umodel.UserDreamArea, error) {
	quests, err := generateQuests(ctx, base, plan)
	if err != nil {
		return nil, err
	}
	area := &umodel.UserDreamArea{SUID: suid, BasePhotoID: base.GetPhotoId(), MaterialPhoto: proto.Clone(material).(*ptransaction.UserPhoto), OriginalAbilities: cloneAbilities(base.GetAbilities()), Quests: quests,
		Data: &papi.DreamUserPhoto{PhotoId: base.GetPhotoId(), GeneratedTime: now, AreaRankType: plan.GetAreaRankType(), AreaRankPlus: plan.GetAreaRankPlus(), MaxQuestNumber: plan.GetMaxQuestNumber(),
			Abilities: clonePlan(plan.GetEnhancePlanAbilities()), AttributeInfo: proto.Clone(plan.GetAttributeInfo()).(*papi.DreamAreaAttributeInfo), RankPoint: plan.GetRankPoint(), MaxGrowthPermil: plan.GetMaxGrowthPermil(), CurrentGrowthPermil: 1000}}
	base.IsDreamBase = true
	base.DreamMaterialPhotoRarity = material.GetRarity()
	base.DreamMaterialPhotoLevel = material.GetLevel()
	base.DreamMaterialPhotoRecipeId = material.GetPhotoRecipeId()
	return area, nil
}

func cloneAbilities(abilities []*ptransaction.UserPhotoAbility) []*ptransaction.UserPhotoAbility {
	result := make([]*ptransaction.UserPhotoAbility, 0, len(abilities))
	for _, a := range abilities {
		result = append(result, proto.Clone(a).(*ptransaction.UserPhotoAbility))
	}
	return result
}
func clonePlan(abilities []*papi.DreamUserPhotoAbility) []*papi.DreamUserPhotoAbility {
	result := make([]*papi.DreamUserPhotoAbility, 0, len(abilities))
	for _, a := range abilities {
		result = append(result, proto.Clone(a).(*papi.DreamUserPhotoAbility))
	}
	return result
}

func generateQuests(ctx context.Context, photo *ptransaction.UserPhoto, plan *papi.DreamUserMaterialPhoto) ([]*papi.DreamQuestInfo, error) {
	masters, err := opmaster.FindAllQuests(ctx)
	if err != nil {
		return nil, err
	}
	pool := make([]*pmaster.Quest, 0)
	for _, q := range masters {
		if strings.HasPrefix(q.GetAreaId(), "area-main-") && q.GetMusicId() != "" && q.GetMusicChartPatternId() != "" && q.GetStageId() != "" {
			pool = append(pool, q)
		}
	}
	if len(pool) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "no main-live assets are available for image training")
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].GetId() < pool[j].GetId() })
	var advantageID string
	if photo.GetFocusCharacterId() != "" {
		settingID, err := config.GetGameSettingId()
		if err != nil {
			return nil, err
		}
		setting, err := opmaster.FindSetting(ctx, settingID)
		if err != nil {
			return nil, err
		}
		id := fmt.Sprintf("quest_character_advantage_%s-%d", photo.GetFocusCharacterId(), setting.GetDreamQuestCharacterAdvantagePermil())
		if _, err := opmaster.FindLiveCharacterAdvantage(ctx, id); err == nil {
			advantageID = id
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(photo.GetPhotoId()))
	seed := h.Sum64()
	count := plan.GetMaxQuestNumber()
	a := plan.GetAttributeInfo()
	maxLevel := int32(50 + plan.GetRankPoint()/40)
	finalScore := 1.4e9 * math.Pow(float64(plan.GetRankPoint())/10670, 3)
	quests := make([]*papi.DreamQuestInfo, 0, count)
	for n := int32(1); n <= count; n++ {
		source := pool[(seed+uint64(n)*7919)%uint64(len(pool))]
		progress := 0.2 + 0.8*float64(n-1)/float64(max(int32(1), count-1))
		level := max(int32(1), int32(float64(maxLevel)*progress))
		q := &papi.DreamQuestInfo{QuestNumber: n, Name: fmt.Sprintf("STAGE%d", n), StageId: source.GetStageId(), MusicId: source.GetMusicId(), MusicChartPatternId: source.GetMusicChartPatternId(), DifficultyLevel: level,
			Position1AttributeType: a.GetAttributeType(), Position2AttributeType: a.GetAttributeType(), Position3AttributeType: a.GetAttributeType(), Position4AttributeType: a.GetAttributeType(), Position5AttributeType: a.GetAttributeType(),
			ActiveSkillWeightPermil: a.GetActiveSkillWeightPermil(), SpecialSkillWeightPermil: a.GetSpecialSkillWeightPermil(), SkillStaminaWeightPermil: a.GetSkillStaminaWeightPermil(), StaminaRecoveryWeightPermil: a.GetStaminaRecoveryWeightPermil(),
			BeatVocalWeightPermil: a.GetBeatVocalWeightPermil(), BeatDanceWeightPermil: a.GetBeatDanceWeightPermil(), BeatVisualWeightPermil: a.GetBeatVisualWeightPermil(),
			MaxCapacity: max(int32(1000), int32(80000*progress*progress)), MentalThreshold: max(int32(100), int32(float64(level)*float64(level)/16)),
			QuestPressureId: a.GetQuestPressureId(), QuestCharacterAdvantageId: advantageID, QuestAudienceAdvantageId: source.GetQuestAudienceAdvantageId(), ClearScore: max(int64(10000), int64(finalScore*math.Pow(progress, 4))), LiveSkipType: penum.LiveSkipType_LiveSkipType_AlwaysFullSkip}
		quests = append(quests, q)
	}
	return quests, nil
}

func ListQuestInfos(area *umodel.UserDreamArea, photo *ptransaction.UserPhoto) []*papi.DreamQuestInfo {
	result := make([]*papi.DreamQuestInfo, 0, len(area.Quests))
	clear := area.Data.GetClearQuestNumber()
	total := area.Data.GetMaxQuestNumber()
	for _, q := range area.Quests {
		row := proto.Clone(q).(*papi.DreamQuestInfo)
		row.Cleared = q.GetQuestNumber() <= clear
		row.IsPlayable = q.GetQuestNumber() == clear+1
		row.IsSkipAvailable = !row.Cleared && q.GetQuestNumber() <= total*4/5
		row.UserPhotoAbilityUpInfos = nil
		if !row.Cleared {
			for i, a := range area.Data.GetAbilities() {
				if i >= len(area.OriginalAbilities) {
					break
				}
				base := area.OriginalAbilities[i].GetEffectValue()
				before := grownValue(base, a.GetMaxEffectValue(), clear, total)
				after := grownValue(base, a.GetMaxEffectValue(), q.GetQuestNumber(), total)
				row.UserPhotoAbilityUpInfos = append(row.UserPhotoAbilityUpInfos, &papi.DreamUserPhotoAbilityUpInfo{PhotoAbilityId: a.GetPhotoAbilityId(), AfterEffectValue: after, UpEffectValue: max(int64(0), after-before)})
			}
		}
		result = append(result, row)
	}
	return result
}

// ApplyProgress uses the saved unenhanced values rather than repeatedly adding
// rounded deltas; multiple calls and resetting cannot compound Dream bonuses.
func ApplyProgress(ctx context.Context, suid string, area *umodel.UserDreamArea, photo *ptransaction.UserPhoto, clearedThrough int32, now int64) ([]*papi.DreamUserPhotoAbilityChangeInfo, error) {
	if area == nil || area.Data == nil || area.SUID != suid || area.BasePhotoID != photo.GetPhotoId() || clearedThrough < area.Data.GetClearQuestNumber() || clearedThrough > area.Data.GetMaxQuestNumber() {
		return nil, status.Error(codes.FailedPrecondition, "invalid image-training progression")
	}
	changes, err := applyPhotoGrowth(area, photo, clearedThrough)
	if err != nil {
		return nil, err
	}
	area.Data.ClearQuestNumber = clearedThrough
	area.Data.LastChallengedTime = now
	if err = opuser.UpsertUserDreamArea(ctx, area); err != nil {
		return nil, err
	}
	if err = opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
		return nil, err
	}
	return changes, nil
}

func applyPhotoGrowth(area *umodel.UserDreamArea, photo *ptransaction.UserPhoto, clear int32) ([]*papi.DreamUserPhotoAbilityChangeInfo, error) {
	if len(area.OriginalAbilities) != len(photo.GetAbilities()) || len(area.Data.GetAbilities()) != len(photo.GetAbilities()) {
		return nil, status.Error(codes.FailedPrecondition, "image-training photo options changed")
	}
	changes := make([]*papi.DreamUserPhotoAbilityChangeInfo, 0, len(photo.GetAbilities()))
	var originalTotal, currentTotal int64
	for i, a := range photo.GetAbilities() {
		original := area.OriginalAbilities[i]
		plan := area.Data.Abilities[i]
		if original.GetPhotoAbilityId() != a.GetPhotoAbilityId() || plan.GetPhotoAbilityId() != a.GetPhotoAbilityId() {
			return nil, status.Error(codes.FailedPrecondition, "image-training photo options changed")
		}
		before := a.GetEffectValue()
		after := grownValue(original.GetEffectValue(), plan.GetMaxEffectValue(), clear, area.Data.GetMaxQuestNumber())
		a.EffectValue = after
		plan.TotalEnhancedEffectValue = after - original.GetEffectValue()
		originalTotal += original.GetEffectValue()
		currentTotal += after
		if after != before {
			changes = append(changes, &papi.DreamUserPhotoAbilityChangeInfo{PhotoAbilityId: a.GetPhotoAbilityId(), BeforeEffectValue: before, AfterEffectValue: after, MaxEffectValue: plan.GetMaxEffectValue(), MaxDisplayValue: plan.GetMaxDisplayValue()})
		}
	}
	area.Data.CurrentGrowthPermil = 1000
	if originalTotal > 0 {
		area.Data.CurrentGrowthPermil = int32(currentTotal * 1000 / originalTotal)
	}
	return changes, nil
}

// RestoreBase is called by ResetArea. Quality upgrades are preserved because
// ReconcilePhotoEnhance replaces this baseline at the new quality beforehand.
func RestoreBase(area *umodel.UserDreamArea, photo *ptransaction.UserPhoto) {
	// Mission availability can change while an area exists. Reset only the
	// enhancement, never a newly unlocked option or its mission association.
	for i, a := range photo.GetAbilities() {
		if i < len(area.OriginalAbilities) && a.GetPhotoAbilityId() == area.OriginalAbilities[i].GetPhotoAbilityId() {
			a.EffectValue = area.OriginalAbilities[i].GetEffectValue()
		}
	}
	photo.IsDreamBase = false
	photo.DreamMaterialPhotoRarity = 0
	photo.DreamMaterialPhotoLevel = 0
	photo.DreamMaterialPhotoRecipeId = ""
}

func ReconcilePhotoEnhance(ctx context.Context, suid string, photo *ptransaction.UserPhoto) error {
	return reconcileEnhance(ctx, suid, photo, true)
}
func ProjectPhotoEnhance(ctx context.Context, suid string, photo *ptransaction.UserPhoto) error {
	return reconcileEnhance(ctx, suid, photo, false)
}

func reconcileEnhance(ctx context.Context, suid string, photo *ptransaction.UserPhoto, persist bool) error {
	if !photo.GetIsDreamBase() {
		return nil
	}
	area, err := opuser.FindUserDreamArea(ctx, suid, photo.GetPhotoId())
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return err
	}
	material := area.MaterialPhoto
	if material == nil {
		return status.Error(codes.FailedPrecondition, "image-training material snapshot is missing")
	}
	material.Level = photo.GetDreamMaterialPhotoLevel()
	masters, err := LoadAbilities(ctx, []*ptransaction.UserPhoto{photo, material})
	if err != nil {
		return err
	}
	for _, a := range material.GetAbilities() {
		value, err := abilityValue(masters[a.GetPhotoAbilityId()], material.GetLevel(), a.GetGrade())
		if err != nil {
			return err
		}
		a.EffectValue = value
	}
	plan, err := Plan(photo, material, masters)
	if err != nil {
		return err
	}
	area.OriginalAbilities = cloneAbilities(photo.GetAbilities())
	area.Data.Abilities = clonePlan(plan.GetEnhancePlanAbilities())
	area.Data.MaxGrowthPermil = plan.GetMaxGrowthPermil()
	if plan.GetMaxQuestNumber() > area.Data.GetMaxQuestNumber() {
		quests, err := generateQuests(ctx, photo, plan)
		if err != nil {
			return err
		}
		area.Quests = append(area.Quests, quests[len(area.Quests):]...)
		area.Data.MaxQuestNumber = plan.GetMaxQuestNumber()
	}
	if _, err = applyPhotoGrowth(area, photo, area.Data.GetClearQuestNumber()); err != nil {
		return err
	}
	if persist {
		return opuser.UpsertUserDreamArea(ctx, area)
	}
	return nil
}

func abilityValue(master *pmaster.PhotoAbility, level, grade int32) (int64, error) {
	var selected *pmaster.PhotoAbilityLevel
	for _, row := range master.GetPhotoAbilityLevels() {
		if row.GetLevel() <= level && (selected == nil || row.GetLevel() > selected.GetLevel()) {
			selected = row
		}
	}
	if selected == nil {
		return 0, status.Error(codes.FailedPrecondition, "photo ability quality is unsupported")
	}
	for _, row := range master.GetPhotoAbilityGrades() {
		if row.GetGrade() == grade {
			return int64(selected.GetValue())*int64(row.GetBonusPermil())/1000 + int64(row.GetBonusValue()), nil
		}
	}
	return 0, status.Error(codes.FailedPrecondition, "photo ability grade is unsupported")
}

func QuestMaster(q *papi.DreamQuestInfo) *pmaster.Quest {
	return &pmaster.Quest{Id: fmt.Sprintf("dream-quest-%d", q.GetQuestNumber()), StageId: q.GetStageId(), MusicId: q.GetMusicId(), Name: q.GetName(), DifficultyLevel: q.GetDifficultyLevel(), MusicChartPatternId: q.GetMusicChartPatternId(),
		Position1AttributeType: q.GetPosition1AttributeType(), Position2AttributeType: q.GetPosition2AttributeType(), Position3AttributeType: q.GetPosition3AttributeType(), Position4AttributeType: q.GetPosition4AttributeType(), Position5AttributeType: q.GetPosition5AttributeType(),
		ActiveSkillWeightPermil: q.GetActiveSkillWeightPermil(), SpecialSkillWeightPermil: q.GetSpecialSkillWeightPermil(), SkillStaminaWeightPermil: q.GetSkillStaminaWeightPermil(), StaminaRecoveryWeightPermil: q.GetStaminaRecoveryWeightPermil(),
		BeatVocalWeightPermil: q.GetBeatVocalWeightPermil(), BeatDanceWeightPermil: q.GetBeatDanceWeightPermil(), BeatVisualWeightPermil: q.GetBeatVisualWeightPermil(), MaxCapacity: q.GetMaxCapacity(), MentalThreshold: q.GetMentalThreshold(),
		QuestPressureId: q.GetQuestPressureId(), QuestCharacterAdvantageId: q.GetQuestCharacterAdvantageId(), QuestAudienceAdvantageId: q.GetQuestAudienceAdvantageId(), ClearScore: q.GetClearScore(), LiveBonusGroupId: q.GetLiveBonusGroupId(), LiveSkipType: q.GetLiveSkipType()}
}
