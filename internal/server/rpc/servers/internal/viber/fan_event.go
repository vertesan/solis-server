package viber

import (
	"context"
	"math"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	FanEventBaseSpecialFanCheerPoint = int32(4)
	FanEventBaseRewardPermyriads     = int32(10000)
	fanEventSpecialFanChanceDenom    = 20 // Round-five traffic is consistent with a 5% roll per idol step.
	FanEventCheerDurationSteps       = int32(20)
	FanEventSuperModeDurationSteps   = int32(25)
)

// NewFanEventRunState creates the non-protobuf state for a new run. The
// cursors begin at zero so background reconciliation and Fetch share the same
// special-fan plan from the first step onward.
func NewFanEventRunState(suid string, progress *papi.FanEventProgress, rewardCoefficientPermyriads int32) *umodel.UserFanEventRunState {
	if rewardCoefficientPermyriads <= 0 {
		rewardCoefficientPermyriads = FanEventBaseRewardPermyriads
	}
	state := &umodel.UserFanEventRunState{
		SUID:                        suid,
		Initialized:                 true,
		RewardCoefficientPermyriads: rewardCoefficientPermyriads,
		PlannedThrough:              make([]umodel.UserFanEventPlannedCursor, 0, len(progress.GetCharacters())),
		SpecialFanSteps:             []umodel.UserFanEventStepReference{},
		CheerEffects:                []umodel.UserFanEventEffectRange{},
		SuperModeEffects:            []umodel.UserFanEventEffectRange{},
	}
	for _, character := range progress.GetCharacters() {
		state.PlannedThrough = append(state.PlannedThrough, umodel.UserFanEventPlannedCursor{CharacterId: character.GetCharacterId()})
	}
	return state
}

// FindFanEventRunState also upgrades runs created before the sidecar existed.
// Existing completed steps become the planning frontier so migration never
// retroactively creates already-consumed special fans.
func FindFanEventRunState(ctx context.Context, suid string, progress *papi.FanEventProgress) (*umodel.UserFanEventRunState, error) {
	state, err := opuser.FindUserFanEventRunState(ctx, suid)
	if err != nil {
		return nil, err
	}
	if state.Initialized {
		if state.RewardCoefficientPermyriads <= 0 {
			state.RewardCoefficientPermyriads = FanEventBaseRewardPermyriads
		}
		return state, nil
	}
	state = NewFanEventRunState(suid, progress, FanEventBaseRewardPermyriads)
	for index, character := range progress.GetCharacters() {
		state.PlannedThrough[index].Step = character.GetCompletedStep()
	}
	return state, nil
}

func FanEventStepDurations(ctx context.Context, progress *papi.FanEventProgress) (foregroundMillis, backgroundMillis int64, err error) {
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return 0, 0, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return 0, 0, err
	}
	speed := max(progress.GetSpeedRate(), 1)
	foregroundMillis = int64(float64(setting.GetSecondsPerActivityFanEventStep()*1000) / speed)
	backgroundMillis = int64(float64(setting.GetSecondsPerActivityFanEventBackgroundStep()*1000) / speed)
	if foregroundMillis <= 0 || backgroundMillis < foregroundMillis {
		return 0, 0, status.Error(codes.FailedPrecondition, "fan event timing is invalid")
	}
	return foregroundMillis, backgroundMillis, nil
}

func EnsureFanEventPlan(state *umodel.UserFanEventRunState, progress *papi.FanEventProgress, characterID string, through int32) error {
	character := fanEventProgressCharacter(progress, characterID)
	if character == nil {
		return status.Error(codes.InvalidArgument, "fan event character is not active")
	}
	through = min(max(int32(0), through), character.GetMaxStep())
	cursor := fanEventPlannedCursor(state, characterID)
	for step := cursor.Step + 1; step <= through; step++ {
		roll, err := CryptoRandomIndex(fanEventSpecialFanChanceDenom)
		if err != nil {
			return err
		}
		if roll == 0 {
			state.SpecialFanSteps = append(state.SpecialFanSteps, umodel.UserFanEventStepReference{
				CharacterId: characterID,
				Step:        step,
			})
		}
	}
	cursor.Step = max(cursor.Step, through)
	return nil
}

func FanEventStepHasSpecialFan(state *umodel.UserFanEventRunState, characterID string, step int32) bool {
	for _, planned := range state.SpecialFanSteps {
		if planned.CharacterId == characterID && planned.Step == step {
			return true
		}
	}
	return false
}

func FanEventStepHasSuperMode(state *umodel.UserFanEventRunState, characterID string, step int32) bool {
	return FanEventEffectAt(state.SuperModeEffects, characterID, step) != nil
}

func ReconcileFanEventProgress(
	ctx context.Context,
	suid string,
	progress *papi.FanEventProgress,
	state *umodel.UserFanEventRunState,
	now int64,
) (bool, error) {
	_, backgroundMillis, err := FanEventStepDurations(ctx, progress)
	if err != nil {
		return false, err
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return false, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return false, err
	}
	superModeValue, err := FanEventSuperModeValue(ctx)
	if err != nil {
		return false, err
	}
	changed := false
	for _, activityCharacter := range progress.GetCharacters() {
		remaining := max(int32(0), activityCharacter.GetMaxStep()-activityCharacter.GetCompletedStep())
		anchor := activityCharacter.GetFinishTime() - int64(remaining)*backgroundMillis
		allowed := min(
			activityCharacter.GetMaxStep(),
			activityCharacter.GetCompletedStep()+int32(max(int64(0), now-anchor)/backgroundMillis),
		)
		if allowed <= activityCharacter.GetCompletedStep() {
			continue
		}
		if err := EnsureFanEventPlan(state, progress, activityCharacter.GetCharacterId(), allowed); err != nil {
			return false, err
		}
		userCharacter, err := opuser.FindUserCharacter(ctx, suid, activityCharacter.GetCharacterId())
		if err != nil {
			return false, err
		}
		basePoint, err := fanEventBasePoint(ctx, userCharacter, progress.GetStepActivityPointCoefficientPermil())
		if err != nil {
			return false, err
		}
		before := activityCharacter.GetCompletedStep()
		var pointGain int64
		for step := before + 1; step <= allowed; step++ {
			// The event profile coefficient defines the maximum well-timed tap.
			// Idle progress receives the Setting intermediate coefficient; e.g.
			// 218 * 17600 / 1000 * 850 / 1000 = the round-five 3260 baseline.
			point := int64(basePoint) * int64(setting.GetActivityFanEventIntermediatePointCoefficientPermil()) / 1000
			point = max(int64(1), point)
			if FanEventEffectAt(state.CheerEffects, activityCharacter.GetCharacterId(), step) != nil {
				point = point * int64(1000+setting.GetActivityFanEventCheerEffectValuePermil()) / 1000
			}
			if FanEventStepHasSuperMode(state, activityCharacter.GetCharacterId(), step) {
				point = point * int64(1000+superModeValue) / 1000
			}
			pointGain = min(int64(math.MaxInt32), pointGain+point)
			if FanEventStepHasSpecialFan(state, activityCharacter.GetCharacterId(), step) {
				progress.CurrentCheerPoint = saturatingFanEventInt32Add(progress.GetCurrentCheerPoint(), progress.GetSpecialFanCheerPoint())
				progress.NewSpecialFansCount = saturatingFanEventInt32Add(progress.GetNewSpecialFansCount(), 1)
			}
		}
		delta := allowed - before
		activityCharacter.CompletedStep = allowed
		activityCharacter.ActivityStamina = max(int32(0), activityCharacter.GetActivityStamina()-delta*progress.GetStepConsumptionStamina())
		activityCharacter.ActivityPoint = saturatingFanEventInt32Add(activityCharacter.GetActivityPoint(), int32(pointGain))
		changed = true
	}
	return changed, nil
}

func SyncActiveFanEventInfo(info *papi.FanEventInfo, progress *papi.FanEventProgress) {
	finish := int64(0)
	ids := make([]string, 0, len(progress.GetCharacters()))
	for _, character := range progress.GetCharacters() {
		finish = max(finish, character.GetFinishTime())
		ids = append(ids, character.GetCharacterId())
	}
	info.ActiveFanEvent = &papi.ActiveFanEvent{
		CharacterIds: ids,
		Name:         progress.GetActivityFanEventName(), SubName: progress.GetActivityFanEventSubName(),
		Level: progress.GetActivityFanEventLevel(), FinishTime: finish,
		CurrentCheerPoint: progress.GetCurrentCheerPoint(),
	}
}

func fanEventPlannedCursor(state *umodel.UserFanEventRunState, characterID string) *umodel.UserFanEventPlannedCursor {
	for index := range state.PlannedThrough {
		if state.PlannedThrough[index].CharacterId == characterID {
			return &state.PlannedThrough[index]
		}
	}
	state.PlannedThrough = append(state.PlannedThrough, umodel.UserFanEventPlannedCursor{CharacterId: characterID})
	return &state.PlannedThrough[len(state.PlannedThrough)-1]
}

func fanEventProgressCharacter(progress *papi.FanEventProgress, characterID string) *papi.FanEventCharacterInfo {
	for _, character := range progress.GetCharacters() {
		if character.GetCharacterId() == characterID {
			return character
		}
	}
	return nil
}

func FanEventEffectAt(effects []umodel.UserFanEventEffectRange, characterID string, step int32) *umodel.UserFanEventEffectRange {
	for index := range effects {
		effect := &effects[index]
		if effect.TargetCharacterId == characterID && step >= effect.StartStep && step <= effect.EndStep {
			return effect
		}
	}
	return nil
}

func FanEventSuperModeValue(ctx context.Context) (int32, error) {
	item, err := opmaster.FindItem(ctx, "item-act_super_mode")
	if err != nil {
		return 0, err
	}
	if item.GetType() != penum.ItemType_ItemType_ActivityFanEventSuperMode || item.GetValue() <= 0 {
		return 0, status.Error(codes.FailedPrecondition, "fan event super mode item master is invalid")
	}
	return item.GetValue(), nil
}

func fanEventBasePoint(ctx context.Context, character *ptransaction.UserCharacter, coefficient int32) (int32, error) {
	level := max(character.GetActivityLevel(), int32(1))
	master, err := opmaster.FindCharacterActivityLevel(ctx, int64(level))
	if err != nil {
		return 0, err
	}
	point := int64(master.GetBaseActivityPoint()) * int64(coefficient) / 1000
	return int32(min(int64(math.MaxInt32), max(int64(1), point))), nil
}

func saturatingFanEventInt32Add(value, delta int32) int32 {
	return int32(min(int64(math.MaxInt32), int64(value)+int64(delta)))
}
