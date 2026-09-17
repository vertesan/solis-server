package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.mongodb.org/mongo-driver/v2/bson"
	"regexp"
)

// Live preparation fetches immutable definitions in batches, never per beat.
func FindLiveChart(ctx context.Context, id string) ([]*pmaster.MusicChartPattern, error) {
	return mcommon.FindMany[mmodel.MusicChartPattern, pmaster.MusicChartPattern](ctx, mname.MusicChartPattern, bson.M{"Id": id})
}

func FindLivePressure(ctx context.Context, id string) (*pmaster.QuestPressure, error) {
	return mcommon.FindOne[mmodel.QuestPressure, pmaster.QuestPressure](ctx, mname.QuestPressure, bson.M{"Id": id})
}

func FindLiveAudienceAdvantages(ctx context.Context, id string) ([]*pmaster.QuestAudienceAdvantage, error) {
	return mcommon.FindMany[mmodel.QuestAudienceAdvantage, pmaster.QuestAudienceAdvantage](ctx, mname.QuestAudienceAdvantage, bson.M{"Id": id})
}

func FindLiveCharacterAdvantage(ctx context.Context, id string) (*pmaster.QuestCharacterAdvantage, error) {
	return mcommon.FindOne[mmodel.QuestCharacterAdvantage, pmaster.QuestCharacterAdvantage](ctx, mname.QuestCharacterAdvantage, bson.M{"Id": id})
}

func FindLivePhotoTargets(ctx context.Context, ids []string) ([]*pmaster.PhotoAbilityTarget, error) {
	return mcommon.FindMany[mmodel.PhotoAbilityTarget, pmaster.PhotoAbilityTarget](ctx, mname.PhotoAbilityTarget, bson.M{"Id": bson.M{"$in": ids}})
}

func FindLiveComboAdvantages(ctx context.Context, id string) ([]*pmaster.ComboAdvantage, error) {
	return mcommon.FindMany[mmodel.ComboAdvantage, pmaster.ComboAdvantage](ctx, mname.ComboAdvantage, bson.M{"Id": id})
}

func FindLiveStaminaConsumption(ctx context.Context) ([]*pmaster.QuestStaminaConsumption, error) {
	return mcommon.FindMany[mmodel.QuestStaminaConsumption, pmaster.QuestStaminaConsumption](ctx, mname.QuestStaminaConsumption, bson.M{})
}

func FindLiveSkills(ctx context.Context, ids []string) ([]*pmaster.Skill, error) {
	return mcommon.FindMany[mmodel.Skill, pmaster.Skill](ctx, mname.Skill, bson.M{"Id": bson.M{"$in": ids}})
}

func FindLiveEfficacies(ctx context.Context, ids []string) ([]*pmaster.SkillEfficacy, error) {
	return mcommon.FindMany[mmodel.SkillEfficacy, pmaster.SkillEfficacy](ctx, mname.SkillEfficacy, bson.M{"Id": bson.M{"$in": ids}})
}

// Destination status names are encoded in conversion efficacy IDs. A single
// destination definition supplies that status's master caps without loading the
// complete efficacy catalog (the query uses the indexed Id prefix).
func FindLiveStatusEfficacy(ctx context.Context, statusName string) (*pmaster.SkillEfficacy, error) {
	return mcommon.FindOne[mmodel.SkillEfficacy, pmaster.SkillEfficacy](ctx, mname.SkillEfficacy, bson.M{"Id": bson.M{"$regex": "^" + regexp.QuoteMeta("ef-"+statusName+"-")}})
}

func FindLiveTriggers(ctx context.Context, ids []string) ([]*pmaster.SkillTrigger, error) {
	return mcommon.FindMany[mmodel.SkillTrigger, pmaster.SkillTrigger](ctx, mname.SkillTrigger, bson.M{"Id": bson.M{"$in": ids}})
}

func FindLiveTargets(ctx context.Context, ids []string) ([]*pmaster.SkillTarget, error) {
	return mcommon.FindMany[mmodel.SkillTarget, pmaster.SkillTarget](ctx, mname.SkillTarget, bson.M{"Id": bson.M{"$in": ids}})
}

func FindLiveAbilities(ctx context.Context, ids []string) ([]*pmaster.LiveAbility, error) {
	return mcommon.FindMany[mmodel.LiveAbility, pmaster.LiveAbility](ctx, mname.LiveAbility, bson.M{"Id": bson.M{"$in": ids}})
}

func FindLiveBonusGroup(ctx context.Context, id string) (*pmaster.LiveBonusGroup, error) {
	return mcommon.FindOne[mmodel.LiveBonusGroup, pmaster.LiveBonusGroup](ctx, mname.LiveBonusGroup, bson.M{"GroupId": id})
}

func FindLiveBonuses(ctx context.Context, ids []string) ([]*pmaster.LiveBonus, error) {
	return mcommon.FindMany[mmodel.LiveBonus, pmaster.LiveBonus](ctx, mname.LiveBonus, bson.M{"Id": bson.M{"$in": ids}})
}

func FindLiveCards(ctx context.Context, ids []string) ([]*pmaster.Card, error) {
	return mcommon.FindMany[mmodel.Card, pmaster.Card](ctx, mname.Card, bson.M{"Id": bson.M{"$in": ids}})
}
