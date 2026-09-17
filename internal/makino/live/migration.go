package live

import (
	"fmt"

	"github.com/vertesan/solis-server/internal/proto/penum"
)

// Migration relocates effects in time, not between idols. A recipient's
// strengths are suspended with their remaining lifetime until its next A/SP
// opportunity, including a transferred chance received from another lane.
// Round5 Quest64 and Gvg46/167/177 show the temporal relocation distinction.
type statusMigration struct {
	category penum.SkillCategoryType
	effect   *statusInstance
}

func applyMigration(e *simulation, source, target *actor, skill *skillState, spec effectSpec) int64 {
	category := penum.SkillCategoryType_SkillCategoryType_Active
	if spec.typ == penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectMigrationBeforeSpecialSkill {
		category = penum.SkillCategoryType_SkillCategoryType_Special
	}
	kept := target.effects[:0]
	for _, effect := range target.effects {
		if e.isStrength(effect.spec.status) {
			// Restored provenance belongs to the relocation skill, not the
			// original applicator (Quest64, charts 67 -> 81/107).
			effect.source, effect.skill = source.index, skill.ref.Index
			target.migrations = append(target.migrations, statusMigration{category: category, effect: effect})
		} else {
			kept = append(kept, effect)
		}
	}
	clear(target.effects[len(kept):])
	target.effects = kept
	target.revision++
	return 0
}

func (e *simulation) restoreMigratedStatuses(a *actor, category penum.SkillCategoryType) {
	kept := a.migrations[:0]
	for _, migration := range a.migrations {
		if migration.category != category {
			kept = append(kept, migration)
			continue
		}
		if len(a.effects) >= maxStatusInstances {
			e.err = fmt.Errorf("too many restored live statuses")
			return
		}
		a.effects = append(a.effects, migration.effect)
		a.revision++
	}
	clear(a.migrations[len(kept):])
	a.migrations = kept
}

// migrationPreview checks whether an idol could use the opportunity after its
// pending effects return, without consuming that opportunity on rejected idols.
func (e *simulation) migrationPreview(a *actor, category penum.SkillCategoryType) *actor {
	count := 0
	for _, migration := range a.migrations {
		if migration.category == category {
			count++
		}
	}
	if count == 0 {
		return a
	}
	preview := *a
	preview.effects = make([]*statusInstance, len(a.effects), len(a.effects)+count)
	copy(preview.effects, a.effects)
	preview.summary = nil
	for _, migration := range a.migrations {
		if migration.category == category {
			preview.effects = append(preview.effects, migration.effect)
		}
	}
	return &preview
}
