package activity

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ActivityServer) SaveFanEvent(ctx context.Context, req *papi.ActivitySaveFanEventRequest) (*papi.ActivitySaveFanEventResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivitySaveFanEventResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserFanEventRun(ctx, suid)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.FailedPrecondition, "no fan event is active")
			}
			return nil, err
		}
		if len(req.GetCharacterResults()) != len(progress.GetCharacters()) {
			return nil, status.Error(codes.InvalidArgument, "one result is required per fan event character")
		}
		state, err := viber.FindFanEventRunState(ctx, suid, progress)
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		if req.GetUseItemId() != "" {
			if progress.GetUsedSuperModeItem() {
				return nil, status.Error(codes.FailedPrecondition, "super mode item was already used")
			}
			if req.GetUseItemId() != fanEventSuperModeItemID {
				return nil, status.Error(codes.InvalidArgument, "unknown fan event super mode item")
			}
			item, err := opmaster.FindItem(ctx, req.GetUseItemId())
			if err != nil {
				return nil, err
			}
			if item.GetType() != penum.ItemType_ItemType_ActivityFanEventSuperMode {
				return nil, status.Error(codes.InvalidArgument, "item cannot activate fan event super mode")
			}
			if _, err := viber.ConsumeExchangeItemStacks(ctx, suid, item.GetId(), 1, updated); err != nil {
				if errors.Is(err, viber.ErrExchangeInsufficientResource) {
					return nil, status.Error(codes.ResourceExhausted, "super mode item is insufficient")
				}
				return nil, err
			}
		}
		resultByID := make(map[string]*papi.FanEventCharacterResult, len(req.GetCharacterResults()))
		for _, result := range req.GetCharacterResults() {
			if result == nil || result.GetCharacterId() == "" {
				return nil, status.Error(codes.InvalidArgument, "fan event character result is invalid")
			}
			if _, duplicate := resultByID[result.GetCharacterId()]; duplicate {
				return nil, status.Error(codes.InvalidArgument, "duplicate fan event character result")
			}
			resultByID[result.GetCharacterId()] = result
		}
		now := time.Now().UnixMilli()
		settingID, err := config.GetGameSettingId()
		if err != nil {
			return nil, err
		}
		setting, err := opmaster.FindSetting(ctx, settingID)
		if err != nil {
			return nil, err
		}
		foregroundMillis, backgroundMillis, err := viber.FanEventStepDurations(ctx, progress)
		if err != nil {
			return nil, err
		}
		// Loading a progress snapshot presents all accumulated special fans to
		// the client. Any subsequent Save acknowledges that displayed batch.
		progress.NewSpecialFansCount = 0
		latestFinishBefore := int64(0)
		for _, character := range progress.GetCharacters() {
			latestFinishBefore = max(latestFinishBefore, character.GetFinishTime())
		}
		for _, character := range progress.GetCharacters() {
			result, ok := resultByID[character.GetCharacterId()]
			if !ok {
				return nil, status.Error(codes.InvalidArgument, "fan event character result does not match active lineup")
			}
			availableMillis := fanEventForegroundElapsedMillis(character, now, backgroundMillis)
			beforeStep := character.GetCompletedStep()
			var reduction int64
			var requiredMillis int64
			if results := result.GetStepResults(); len(results) > 0 {
				if err := viber.EnsureFanEventPlan(
					state, progress, character.GetCharacterId(), results[len(results)-1].GetStep(),
				); err != nil {
					return nil, err
				}
			}
			for _, stepResult := range result.GetStepResults() {
				if stepResult.GetStep() != character.GetCompletedStep()+1 || stepResult.GetStep() > character.GetMaxStep() {
					return nil, status.Error(codes.InvalidArgument, "fan event steps must be sequential and within the activity")
				}
				// Tap timing, fever and cheer composition are calculated by the
				// official client. The server owns step integrity, not point magnitude.
				if stepResult.GetActivityPoint() < 0 || int64(character.GetActivityPoint())+int64(stepResult.GetActivityPoint()) > math.MaxInt32 {
					return nil, status.Error(codes.InvalidArgument, "fan event activity point is invalid")
				}
				maxReduction := int64(0)
				if viber.FanEventStepHasSuperMode(state, character.GetCharacterId(), stepResult.GetStep()) {
					maxReduction = foregroundMillis
				}
				if stepResult.GetReductionMilliSeconds() < 0 || stepResult.GetReductionMilliSeconds() > maxReduction {
					return nil, status.Error(codes.InvalidArgument, "fan event time reduction is invalid")
				}
				// During fever the client can shorten each foreground step through
				// tap timing. Spend the effective duration here instead of checking
				// the whole batch against nominal five-second steps up front.
				stepMillis := foregroundMillis - stepResult.GetReductionMilliSeconds()
				if stepMillis > availableMillis-requiredMillis {
					return nil, status.Error(codes.InvalidArgument, "fan event steps have not elapsed")
				}
				requiredMillis += stepMillis
				character.CompletedStep++
				character.ActivityPoint += stepResult.GetActivityPoint()
				reduction += stepResult.GetReductionMilliSeconds()
				if viber.FanEventStepHasSpecialFan(state, character.GetCharacterId(), stepResult.GetStep()) {
					if int64(progress.GetCurrentCheerPoint())+int64(progress.GetSpecialFanCheerPoint()) > math.MaxInt32 {
						return nil, status.Error(codes.FailedPrecondition, "fan event cheer point overflows")
					}
					progress.CurrentCheerPoint += progress.GetSpecialFanCheerPoint()
				}
			}
			delta := character.GetCompletedStep() - beforeStep
			character.ActivityStamina = max(int32(0), character.GetActivityStamina()-delta*progress.GetStepConsumptionStamina())
			finishReduction := int64(delta)*(backgroundMillis-foregroundMillis) + reduction
			character.FinishTime = max(progress.GetStartTime(), character.GetFinishTime()-finishReduction)
			character.LastSavedActivityPoint = character.GetActivityPoint()
		}
		latestFinish := int64(0)
		for _, character := range progress.GetCharacters() {
			latestFinish = max(latestFinish, character.GetFinishTime())
		}
		if latestFinish < latestFinishBefore {
			updated.Notifications = append(updated.Notifications, &ptransaction.UserNotification{
				NotificationType: penum.NotificationType_NotificationType_Activity,
				Valid:            true,
				StartTime:        latestFinish,
			})
		}
		if req.GetUseItemId() != "" {
			progress.UsedSuperModeItem = true
			required := setting.GetActivityFanEventRequiredCheerPoint()
			if required <= 0 || int64(progress.GetCurrentCheerPoint())+int64(required) > math.MaxInt32 {
				return nil, status.Error(codes.FailedPrecondition, "fan event cheer point is invalid")
			}
			// Fever grants one full gauge directly. Cheer-point-up items do not
			// modify this grant.
			progress.CurrentCheerPoint += required
			addFanEventSuperMode(state, progress)
		}
		if req.GetExecCheer() {
			required := setting.GetActivityFanEventRequiredCheerPoint()
			if required <= 0 || progress.GetCurrentCheerPoint() < required {
				return nil, status.Error(codes.FailedPrecondition, "fan event cheer point is insufficient")
			}
			progress.CurrentCheerPoint -= required
			if err := addFanEventCheer(ctx, suid, state, progress); err != nil {
				return nil, err
			}
		}
		if err := opuser.UpsertUserFanEventRun(ctx, suid, progress); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserFanEventRunState(ctx, state); err != nil {
			return nil, err
		}
		if err := syncActiveFanEvent(ctx, suid, progress); err != nil {
			return nil, err
		}
		rankType, rankPlus := fanEventRank(progress.GetRankPatterns(), fanEventScore(progress))
		best, err := opuser.FindUserActivityFanEventProgress(ctx, suid)
		if err != nil {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return nil, err
			}
			best = &ptransaction.UserActivityFanEventProgress{}
		}
		if rankType > best.GetBestScoreRankType() || (rankType == best.GetBestScoreRankType() && int64(rankPlus) > best.GetBestScoreRankPlus()) {
			best.BestScoreRankType = rankType
			best.BestScoreRankPlus = int64(rankPlus)
		}
		if err := opuser.UpsertUserActivityFanEventProgress(ctx, suid, best); err != nil {
			return nil, err
		}
		updated.ActivityFanEventProgress = best
		if err := updateFanEventRanking(ctx, suid, progress); err != nil {
			return nil, err
		}
		var issuedSteps []*papi.FanEventCharacterSteps
		if req.GetUseItemId() != "" || req.GetExecCheer() {
			stepCursors := make([]int32, 0, len(progress.GetCharacters()))
			for _, character := range progress.GetCharacters() {
				stepCursors = append(stepCursors, min(character.GetMaxStep(), character.GetCompletedStep()+1))
			}
			issuedSteps, err = issueFanEventSteps(ctx, progress, state, stepCursors)
			if err != nil {
				return nil, err
			}
			if err := opuser.UpsertUserFanEventRunState(ctx, state); err != nil {
				return nil, err
			}
		}
		response = &papi.ActivitySaveFanEventResponse{
			CharacterSteps: issuedSteps,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// fanEventForegroundElapsedMillis derives the foreground time available since
// the character's current timing anchor. SaveFanEvent accounts for fever tap
// reductions while spending this budget.
func fanEventForegroundElapsedMillis(character *papi.FanEventCharacterInfo, now, backgroundMillis int64) int64 {
	remaining := max(int32(0), character.GetMaxStep()-character.GetCompletedStep())
	anchor := character.GetFinishTime() - int64(remaining)*backgroundMillis
	return max(int64(0), now-anchor)
}

func addFanEventSuperMode(state *umodel.UserFanEventRunState, progress *papi.FanEventProgress) {
	for _, character := range progress.GetCharacters() {
		start := character.GetCompletedStep() + 1
		end := min(character.GetMaxStep(), start+viber.FanEventSuperModeDurationSteps-1)
		if start <= end {
			state.SuperModeEffects = append(state.SuperModeEffects, umodel.UserFanEventEffectRange{
				TargetCharacterId: character.GetCharacterId(), StartStep: start, EndStep: end,
			})
		}
	}
}

// addFanEventCheer chooses one working idol as the target and a different
// owned idol as supporter. The selected effect is persisted for twenty target
// steps, matching the official round-five sequence.
func addFanEventCheer(ctx context.Context, suid string, state *umodel.UserFanEventRunState, progress *papi.FanEventProgress) error {
	targets := make([]*papi.FanEventCharacterInfo, 0, len(progress.GetCharacters()))
	for _, character := range progress.GetCharacters() {
		if character.GetCompletedStep() < character.GetMaxStep() {
			targets = append(targets, character)
		}
	}
	targetIndex, err := viber.CryptoRandomIndex(len(targets))
	if err != nil {
		return err
	}
	target := targets[targetIndex]
	active := make(map[string]struct{}, len(progress.GetCharacters()))
	for _, character := range progress.GetCharacters() {
		active[character.GetCharacterId()] = struct{}{}
	}
	owned, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		return err
	}
	supporters := make([]string, 0, len(owned))
	for _, character := range owned {
		if _, working := active[character.GetCharacterId()]; !working {
			supporters = append(supporters, character.GetCharacterId())
		}
	}
	if len(supporters) == 0 {
		for _, character := range owned {
			if character.GetCharacterId() != target.GetCharacterId() {
				supporters = append(supporters, character.GetCharacterId())
			}
		}
	}
	if len(supporters) == 0 {
		return status.Error(codes.FailedPrecondition, "no idol is available to cheer")
	}
	supportIndex, err := viber.CryptoRandomIndex(len(supporters))
	if err != nil {
		return err
	}
	start := target.GetCompletedStep() + 1
	end := min(target.GetMaxStep(), start+viber.FanEventCheerDurationSteps-1)
	if start <= end {
		state.CheerEffects = append(state.CheerEffects, umodel.UserFanEventEffectRange{
			TargetCharacterId: target.GetCharacterId(), SupportCharacterId: supporters[supportIndex],
			StartStep: start, EndStep: end,
		})
	}
	return nil
}

const fanEventSuperModeItemID = "item-act_super_mode"
