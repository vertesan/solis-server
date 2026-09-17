package live

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
)

// Captured Start descriptions are empty. These compact Japanese messages are
// private-server explanations of the actual trace, not reproduced official prose.
// Logging reads existing outcomes and never rolls RNG or re-evaluates effects.
func (e *simulation) skillLabel(a *actor, s *skillState) string {
	name := s.definition.Name
	if name == "" {
		name = s.ref.SkillId
	}
	return fmt.Sprintf("位置%d %s", a.index, name)
}

func failureLabel(reason penum.SkillFailureType) string {
	switch reason {
	case penum.SkillFailureType_SkillFailureType_StaminaShortage:
		return "スタミナ不足"
	case penum.SkillFailureType_SkillFailureType_InCoolTime:
		return "CT中"
	case penum.SkillFailureType_SkillFailureType_NoRemainingCount:
		return "発動回数なし"
	case penum.SkillFailureType_SkillFailureType_CategoryMismatch:
		return "対応スキルなし"
	case penum.SkillFailureType_SkillFailureType_InSkillImpossibleEffect:
		return "発動不可状態"
	case penum.SkillFailureType_SkillFailureType_OpponentActivation:
		return "相手が発動権を獲得"
	case penum.SkillFailureType_SkillFailureType_Miss:
		return "発動確率判定に失敗"
	case penum.SkillFailureType_SkillFailureType_AllSkillEfficacyTriggerFail:
		return "効果条件未成立"
	default:
		return reason.String()
	}
}

func (e *simulation) failureDescriptions(rows []*papi.LiveSkillFailure) []*papi.LiveLogDescription {
	out := make([]*papi.LiveLogDescription, 0, len(rows))
	for _, row := range rows {
		reason := failureLabel(row.Type)
		if len(row.EachFailureTypes) > 1 {
			labels := make([]string, 0, len(row.EachFailureTypes))
			seen := map[penum.SkillFailureType]bool{}
			for _, t := range row.EachFailureTypes {
				if !seen[t] {
					labels = append(labels, failureLabel(t))
					seen[t] = true
				}
			}
			reason = strings.Join(labels, " / ")
		}
		out = append(out, &papi.LiveLogDescription{Message: fmt.Sprintf("位置%d: %s（消費なし）", row.CardIndex, reason)})
	}
	return out
}

func (e *simulation) skillDescriptions(a *actor, s *skillState, result *papi.LiveSkill) []*papi.LiveLogDescription {
	if !result.Activated {
		if len(result.Failures) > 0 {
			return e.failureDescriptions(result.Failures)
		}
		return []*papi.LiveLogDescription{{Message: e.skillLabel(a, s) + ": 発動しませんでした（消費なし）"}}
	}
	out := make([]*papi.LiveLogDescription, 0, len(result.Details)+1)
	critical := ""
	if result.IsCritical {
		critical = " / クリティカル"
	}
	out = append(out, &papi.LiveLogDescription{Message: fmt.Sprintf("%s: 発動 / 消費スタミナ%d / スコア%d%s", e.skillLabel(a, s), result.Stamina, result.Score, critical)})
	for _, detail := range result.Details {
		if detail.EfficacyIndex < 1 || int(detail.EfficacyIndex) > len(s.effects) {
			continue
		}
		eff := s.effects[detail.EfficacyIndex-1]
		name := e.input.Efficacies[eff.id].GetName()
		if name == "" {
			name = eff.typ.String()
		}
		var targets strings.Builder
		for i, index := range detail.TargetCardIndexes {
			if i > 0 {
				targets.WriteByte(',')
			}
			targets.WriteString(strconv.FormatInt(int64(index), 10))
		}
		if len(detail.TargetCardIndexes) == 0 {
			if strings.HasPrefix(eff.operation, "score_get") {
				targets.WriteString("自身")
			} else {
				targets.WriteString("なし")
			}
		}
		message := fmt.Sprintf("%s / 対象:%s / 値:%d", name, targets.String(), detail.Value)
		if detail.StatusType != penum.StatusEffectType_StatusEffectType_Unknown {
			message += fmt.Sprintf(" / %s / 基本%d段階 / 基本%dビート", detail.StatusType, eff.grade, detail.EffectiveChartCount)
		}
		out = append(out, &papi.LiveLogDescription{Message: message})
	}
	return out
}
