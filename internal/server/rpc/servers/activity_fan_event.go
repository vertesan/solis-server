package servers

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
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const fanEventSuperModeItemID = "item-act_super_mode"

func (s *ActivityServer) StartFanEvent(ctx context.Context, req *papi.ActivityStartFanEventRequest) (*papi.ActivityStartFanEventResponse, error) {
	if req.GetActivityFanEventId() == "" {
		return nil, status.Error(codes.InvalidArgument, "activityFanEventId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityStartFanEventResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		info, err := opuser.FindUserFanEventInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		now := time.Now().UnixMilli()
		if err := viber.ReconcileFanEventInfo(ctx, suid, now, info); err != nil {
			return nil, err
		}
		if info.GetActiveFanEvent() != nil {
			return nil, status.Error(codes.FailedPrecondition, "a fan event is already active")
		}
		if _, err := opuser.FindUserFanEventRun(ctx, suid); err == nil {
			return nil, status.Error(codes.FailedPrecondition, "a fan event is already active")
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		fanEvent := findUserFanEvent(info, req.GetActivityFanEventId())
		if fanEvent == nil {
			return nil, status.Error(codes.NotFound, "fan event not found")
		}
		if !fanEvent.GetIsUnlocked() {
			return nil, status.Error(codes.PermissionDenied, "fan event is locked")
		}
		master, masterLevel, err := findFanEventMaster(ctx, fanEvent.GetId(), fanEvent.GetLevel())
		if err != nil {
			return nil, err
		}
		characters, lineup, err := viber.ValidateActivityLineup(
			ctx, suid, fanEvent.GetActivityPerformanceId(), req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds(), now,
		)
		if err != nil {
			return nil, err
		}
		settingID, err := config.GetGameSettingId()
		if err != nil {
			return nil, err
		}
		setting, err := opmaster.FindSetting(ctx, settingID)
		if err != nil {
			return nil, err
		}
		stepSeconds := setting.GetSecondsPerActivityFanEventBackgroundStep()
		stepStamina := fanEvent.GetStepConsumptionStamina()
		if stepSeconds <= 0 || stepStamina <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "fan event timing is invalid")
		}
		updated := &papi.UpdatedData{}
		speedRate := 1.0
		specialCheerPoint := viber.FanEventBaseSpecialFanCheerPoint
		rewardCoefficientPermyriads := viber.FanEventBaseRewardPermyriads
		usedTypes := make(map[penum.ItemType]struct{}, len(req.GetItemIds()))
		for _, itemID := range req.GetItemIds() {
			item, err := opmaster.FindItem(ctx, itemID)
			if err != nil {
				return nil, err
			}
			switch item.GetType() {
			case penum.ItemType_ItemType_ActivityFanEventSpeedUp,
				penum.ItemType_ItemType_ActivityFanEventCheerPointUp,
				penum.ItemType_ItemType_ActivityFanEventRewardUp:
			default:
				return nil, status.Error(codes.InvalidArgument, "item cannot be used when starting a fan event")
			}
			if _, duplicate := usedTypes[item.GetType()]; duplicate {
				return nil, status.Error(codes.InvalidArgument, "duplicate fan event item type")
			}
			usedTypes[item.GetType()] = struct{}{}
			if _, err := viber.ConsumeExchangeItemStacks(ctx, suid, itemID, 1, updated); err != nil {
				if errors.Is(err, viber.ErrExchangeInsufficientResource) {
					return nil, status.Error(codes.ResourceExhausted, "fan event item is insufficient")
				}
				return nil, err
			}
			if item.GetType() == penum.ItemType_ItemType_ActivityFanEventSpeedUp && item.GetValue() > 0 {
				// Fan-event item values are additive permil. For example, the
				// official speed item has value 1000 and produces speedRate 2.0.
				speedRate += float64(item.GetValue()) / 1000
			}
			if item.GetType() == penum.ItemType_ItemType_ActivityFanEventCheerPointUp && item.GetValue() > 0 {
				specialCheerPoint = int32(min(
					int64(math.MaxInt32),
					int64(specialCheerPoint)*int64(1000+item.GetValue())/1000,
				))
			}
			if item.GetType() == penum.ItemType_ItemType_ActivityFanEventRewardUp && item.GetValue() > 0 {
				rewardCoefficientPermyriads = int32(min(
					int64(math.MaxInt32),
					int64(rewardCoefficientPermyriads)+int64(item.GetValue()),
				))
			}
		}
		coefficient := int32(1000)
		balanceProfile, err := viber.FindFanEventBalanceProfile(fanEvent.GetId(), fanEvent.GetLevel())
		if err != nil {
			return nil, err
		}
		if balanceProfile != nil {
			coefficient = balanceProfile.ActivityPointCoefficientPermil
		}
		progressCharacters := make([]*papi.FanEventCharacterInfo, 0, len(characters))
		var maximumScore int64
		var latestFinish int64
		for index, character := range characters {
			if character.GetActivityStamina() < fanEvent.GetRequiredStamina() {
				return nil, status.Error(codes.FailedPrecondition, "character stamina is insufficient")
			}
			maxStep := character.GetActivityStamina() / stepStamina
			if maxStep <= 0 {
				return nil, status.Error(codes.FailedPrecondition, "character stamina is insufficient")
			}
			stepMillis := int64(float64(stepSeconds*1000) / speedRate)
			if stepMillis <= 0 {
				return nil, status.Error(codes.FailedPrecondition, "fan event speed is invalid")
			}
			finish := now + int64(maxStep)*stepMillis
			latestFinish = max(latestFinish, finish)
			basePoint, err := fanEventBasePoint(ctx, character, coefficient)
			if err != nil {
				return nil, err
			}
			maximumScore += int64(maxStep) * int64(basePoint)
			progressCharacters = append(progressCharacters, &papi.FanEventCharacterInfo{
				CharacterId:     character.GetCharacterId(),
				CostumeId:       lineup[index].GetCostumeId(),
				HairId:          lineup[index].GetHairId(),
				ActivityStamina: character.GetActivityStamina(),
				FinishTime:      finish,
				MaxStep:         maxStep,
			})
			character.InActivity = true
			character.StaminaUpdatedTime = now
			if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
				return nil, err
			}
			updated.Characters = append(updated.Characters, character)
			missions, err := viber.AdvanceUserMissions(
				ctx, suid, penum.MissionType_MissionType_CharacterStartActivityFanEventCount, 1, character.GetCharacterId(),
			)
			if err != nil {
				return nil, err
			}
			updated.Missions = append(updated.Missions, missions...)
		}
		progress := &papi.FanEventProgress{
			ActivityFanEventId:                 master.GetId(),
			ActivityFanEventName:               master.GetName(),
			ActivityFanEventSubName:            masterLevel.GetName(),
			ActivityFanEventType:               master.GetType(),
			ActivityFanEventLevel:              fanEvent.GetLevel(),
			Characters:                         progressCharacters,
			StartTime:                          now,
			StepConsumptionStamina:             stepStamina,
			StepActivityPointCoefficientPermil: coefficient,
			RankPatterns:                       nil,
			ActivityPerformanceId:              fanEvent.GetActivityPerformanceId(),
			MaleNpcSpineId:                     "female-00",
			FemaleNpcSpineId:                   "male-00",
			SpecialFanCheerPoint:               specialCheerPoint,
			SpeedRate:                          speedRate,
		}
		rankAThreshold := maximumScore
		if balanceProfile != nil {
			rankAThreshold = balanceProfile.RankAThresholdPoint
		}
		progress.RankPatterns = fanEventRankPatterns(rankAThreshold)
		if err := opuser.UpsertUserFanEventRun(ctx, suid, progress); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserFanEventRunState(
			ctx, viber.NewFanEventRunState(suid, progress, rewardCoefficientPermyriads),
		); err != nil {
			return nil, err
		}
		info.ActiveFanEvent = &papi.ActiveFanEvent{
			CharacterIds: append([]string(nil), req.GetCharacterIds()...),
			Name:         progress.GetActivityFanEventName(),
			SubName:      progress.GetActivityFanEventSubName(),
			Level:        progress.GetActivityFanEventLevel(),
			FinishTime:   latestFinish,
		}
		if err := opuser.UpsertUserFanEventInfo(ctx, suid, info); err != nil {
			return nil, err
		}
		response = &papi.ActivityStartFanEventResponse{Progress: progress, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (s *ActivityServer) LoadFanEvent(ctx context.Context, _ *papi.Empty) (*papi.ActivityLoadFanEventResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityLoadFanEventResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserFanEventRun(ctx, suid)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.FailedPrecondition, "no fan event is active")
			}
			return nil, err
		}
		state, err := viber.FindFanEventRunState(ctx, suid, progress)
		if err != nil {
			return nil, err
		}
		if _, err := viber.ReconcileFanEventProgress(ctx, suid, progress, state, time.Now().UnixMilli()); err != nil {
			return nil, err
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
		response = &papi.ActivityLoadFanEventResponse{Progress: progress, CommonResponse: viber.NewCommonResponse(nil)}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (s *ActivityServer) FetchFanEventSteps(ctx context.Context, req *papi.ActivityFetchFanEventStepsRequest) (*papi.ActivityFetchFanEventStepsResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityFetchFanEventStepsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserFanEventRun(ctx, suid)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.FailedPrecondition, "no fan event is active")
			}
			return nil, err
		}
		if len(req.GetSteps()) != len(progress.GetCharacters()) {
			return nil, status.Error(codes.InvalidArgument, "one step cursor is required per character")
		}
		state, err := viber.FindFanEventRunState(ctx, suid, progress)
		if err != nil {
			return nil, err
		}
		characterSteps, err := viber.IssueFanEventSteps(ctx, progress, state, req.GetSteps())
		if err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserFanEventRunState(ctx, state); err != nil {
			return nil, err
		}
		response = &papi.ActivityFetchFanEventStepsResponse{
			CharacterSteps: characterSteps,
			CommonResponse: viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

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
			availableMillis := viber.FanEventForegroundElapsedMillis(character, now, backgroundMillis)
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
			viber.AddFanEventSuperMode(state, progress)
		}
		if req.GetExecCheer() {
			required := setting.GetActivityFanEventRequiredCheerPoint()
			if required <= 0 || progress.GetCurrentCheerPoint() < required {
				return nil, status.Error(codes.FailedPrecondition, "fan event cheer point is insufficient")
			}
			progress.CurrentCheerPoint -= required
			if err := viber.AddFanEventCheer(ctx, suid, state, progress); err != nil {
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
			issuedSteps, err = viber.IssueFanEventSteps(ctx, progress, state, stepCursors)
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

func (s *ActivityServer) FinishFanEvent(ctx context.Context, _ *papi.Empty) (*papi.ActivityFinishFanEventResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityFinishFanEventResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserFanEventRun(ctx, suid)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.FailedPrecondition, "no fan event is active")
			}
			return nil, err
		}
		now := time.Now().UnixMilli()
		state, err := viber.FindFanEventRunState(ctx, suid, progress)
		if err != nil {
			return nil, err
		}
		if _, err := viber.ReconcileFanEventProgress(ctx, suid, progress, state, now); err != nil {
			return nil, err
		}
		for _, character := range progress.GetCharacters() {
			if character.GetMaxStep() <= 0 || character.GetCompletedStep() != character.GetMaxStep() {
				return nil, status.Error(codes.FailedPrecondition, "fan event is not complete")
			}
		}
		info, err := opuser.FindUserFanEventInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		fanEvent := findUserFanEvent(info, progress.GetActivityFanEventId())
		if fanEvent == nil {
			return nil, status.Error(codes.FailedPrecondition, "active fan event catalog entry is missing")
		}
		score := fanEventScore(progress)
		rankType, rankPlus := fanEventRank(progress.GetRankPatterns(), score)
		updated := &papi.UpdatedData{}
		rewards, err := viber.ResolveFanEventRewards(
			ctx, progress, fanEvent, rankType, state.RewardCoefficientPermyriads,
		)
		if err != nil {
			return nil, err
		}
		for _, reward := range rewards {
			if err := viber.GrantActivityReward(ctx, suid, reward, updated); err != nil {
				return nil, err
			}
		}
		balanceProfile, err := viber.FindFanEventBalanceProfile(fanEvent.GetId(), fanEvent.GetLevel())
		if err != nil {
			return nil, err
		}
		coreFanCoefficient, err := viber.FanEventCoreFanConversionMillionths(
			balanceProfile, fanEventRankKey(rankType),
		)
		if err != nil {
			return nil, err
		}
		selected := make([]*papi.ActivityCharacterInfo, 0, len(progress.GetCharacters()))
		for _, activityCharacter := range progress.GetCharacters() {
			character, err := opuser.FindUserCharacter(ctx, suid, activityCharacter.GetCharacterId())
			if err != nil {
				return nil, err
			}
			// ActivityStamina in the run is a post-image during normal saves. Derive
			// it from completed steps as well so a background-completed run cannot
			// retain the start-time stamina value.
			remainder := activityCharacter.GetActivityStamina() % progress.GetStepConsumptionStamina()
			character.ActivityStamina = max(
				int32(0),
				(activityCharacter.GetMaxStep()-activityCharacter.GetCompletedStep())*progress.GetStepConsumptionStamina()+remainder,
			)
			character.StaminaUpdatedTime = now
			character.InActivity = false
			characterActivityExp := int64(activityCharacter.GetCompletedStep())
			if balanceProfile != nil {
				characterActivityExp = balanceProfile.CharacterActivityExp
			}
			if characterActivityExp > 0 && character.GetActivityExp() > math.MaxInt64-characterActivityExp {
				return nil, status.Error(codes.FailedPrecondition, "character activity exp overflows")
			}
			character.ActivityExp += characterActivityExp
			if err := advanceCharacterActivityLevel(ctx, character); err != nil {
				return nil, err
			}
			if err := applyFanEventFanConversion(character, coreFanCoefficient); err != nil {
				return nil, err
			}
			if err := viber.RecalculateAudienceCandidateAmount(ctx, character); err != nil {
				return nil, err
			}
			if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
				return nil, err
			}
			updated.Characters = append(updated.Characters, character)
			selected = append(selected, &papi.ActivityCharacterInfo{
				CharacterId: activityCharacter.GetCharacterId(),
				CostumeId:   activityCharacter.GetCostumeId(),
				HairId:      activityCharacter.GetHairId(),
			})
		}
		before := &papi.ActivityLevelInfo{
			Exp:             fanEvent.GetExp(),
			Level:           fanEvent.GetLevel(),
			NextLevelLocked: fanEventNextLevelLocked(fanEvent, fanEvent.GetExp()),
		}
		expGain := int32(min(
			int64(math.MaxInt32),
			int64(totalFanEventSteps(progress))*3,
		))
		fanEvent.Exp = int32(min(int64(math.MaxInt32), int64(fanEvent.GetExp())+int64(expGain)))
		if err := replaceUserFanEvent(info, fanEvent); err != nil {
			return nil, err
		}
		info.ActiveFanEvent = nil
		if err := opuser.UpsertUserFanEventInfo(ctx, suid, info); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserFanEventRun(ctx, suid); err != nil {
			return nil, err
		}
		total, err := opuser.FindUserTotalCount(ctx, suid)
		if err != nil {
			return nil, err
		}
		total.ActivityFanEventCount++
		if err := opuser.UpsertUserTotalCount(ctx, suid, total); err != nil {
			return nil, err
		}
		updated.TotalCount = total
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
		maxLevel, err := viber.MaxUnlockedFanEventLevel(ctx, suid, fanEvent.GetId(), now, fanEvent.GetLevel())
		if err != nil {
			return nil, err
		}
		nextLevelLocked := fanEventNextLevelLocked(fanEvent, fanEvent.GetExp())
		response = &papi.ActivityFinishFanEventResponse{
			ActivityPoint: int32(min(int64(math.MaxInt32), score)),
			RankType:      rankType,
			Plus:          rankPlus,
			Rewards:       rewards,
			RankPatterns:  progress.GetRankPatterns(),
			ActivityLevelProgress: &papi.ActivityLevelProgress{
				MaxActivityLevel: maxLevel,
				Before:           before,
				After:            &papi.ActivityLevelInfo{Exp: fanEvent.GetExp(), Level: fanEvent.GetLevel(), NextLevelLocked: nextLevelLocked},
			},
			Characters:          selected,
			CampaignEffects:     []*papi.ActivityCampaignEffect{},
			CampaignDropRewards: []*ptransaction.Reward{},
			CommonResponse:      viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (s *ActivityServer) GetFanEventRanking(ctx context.Context, req *papi.ActivityGetFanEventRankingRequest) (*papi.ActivityGetFanEventRankingResponse, error) {
	if req.GetActivityFanEventId() == "" {
		return nil, status.Error(codes.InvalidArgument, "activityFanEventId is required")
	}
	suid := viber.GetSuid(ctx).String()
	info, err := opuser.FindUserFanEventInfo(ctx, suid)
	if err != nil {
		return nil, err
	}
	if findUserFanEvent(info, req.GetActivityFanEventId()) == nil {
		return nil, status.Error(codes.NotFound, "fan event not found")
	}
	ranking, err := opuser.FindUserFanEventRanking(ctx, suid, req.GetActivityFanEventId())
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		ranking = &umodel.UserFanEventRanking{SUID: suid, ActivityFanEventId: req.GetActivityFanEventId(), CharacterIds: []string{}}
	}
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, err
	}
	public, err := opuser.FindUserPublic(ctx, suid)
	if err != nil {
		return nil, err
	}
	rankInfo := &papi.FanEventRankingInfo{
		Rank:         1,
		Score:        ranking.BestScore,
		UserId:       public.GetPublicUserId(),
		Name:         user.GetName(),
		ManagerLevel: user.GetManagerLevel(),
		CharacterIds: append([]string(nil), ranking.CharacterIds...),
		EmblemId:     user.GetEmblemId(),
	}
	return &papi.ActivityGetFanEventRankingResponse{
		SelfRank:       1,
		SelfScore:      ranking.BestScore,
		RankInfos:      []*papi.FanEventRankingInfo{rankInfo},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func findUserFanEvent(info *papi.FanEventInfo, id string) *papi.FanEvent {
	for _, fanEvent := range info.GetFanEvents() {
		if fanEvent.GetId() == id {
			return fanEvent
		}
	}
	return nil
}

func replaceUserFanEvent(info *papi.FanEventInfo, fanEvent *papi.FanEvent) error {
	for index, current := range info.GetFanEvents() {
		if current.GetId() == fanEvent.GetId() {
			info.FanEvents[index] = fanEvent
			return nil
		}
	}
	return status.Error(codes.FailedPrecondition, "fan event catalog entry is missing")
}

func findFanEventMaster(ctx context.Context, id string, level int32) (*pmaster.ActivityFanEvent, *pmaster.ActivityFanEventLevel, error) {
	masters, err := opmaster.FindAllActivityFanEvents(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, master := range masters {
		if master.GetId() != id {
			continue
		}
		for _, masterLevel := range master.GetLevels() {
			if masterLevel.GetLevel() == level {
				return master, masterLevel, nil
			}
		}
		return nil, nil, status.Error(codes.FailedPrecondition, "fan event level is missing from master data")
	}
	return nil, nil, status.Error(codes.NotFound, "fan event is missing from master data")
}

func fanEventBasePoint(ctx context.Context, character *ptransaction.UserCharacter, coefficient int32) (int32, error) {
	level := character.GetActivityLevel()
	if level <= 0 {
		level = 1
	}
	master, err := opmaster.FindCharacterActivityLevel(ctx, int64(level))
	if err != nil {
		return 0, err
	}
	point := int64(master.GetBaseActivityPoint()) * int64(coefficient) / 1000
	if point <= 0 {
		point = 1
	}
	return int32(min(int64(math.MaxInt32), point)), nil
}

func fanEventRankPatterns(maximumScore int64) []*papi.FanEventRankPattern {
	maximumScore = max(maximumScore, 1)
	patterns := make([]*papi.FanEventRankPattern, 0, 25)
	for plus := int32(20); plus >= 1; plus-- {
		percentage := int64(180)
		if plus >= 2 {
			percentage += int64(plus) * 10
		}
		threshold := maximumScore * percentage / 100
		patterns = append(patterns, &papi.FanEventRankPattern{
			RankType: penum.ResultRankType_ResultRankType_Splus,
			Plus:     plus, ThresholdPoint: int32(min(int64(math.MaxInt32), threshold)),
		})
	}
	for _, pattern := range []struct {
		rank      penum.ResultRankType
		threshold int64
	}{
		{penum.ResultRankType_ResultRankType_S, maximumScore * 140 / 100},
		{penum.ResultRankType_ResultRankType_A, maximumScore},
		{penum.ResultRankType_ResultRankType_B, maximumScore * 60 / 100},
		{penum.ResultRankType_ResultRankType_C, maximumScore * 30 / 100},
		{penum.ResultRankType_ResultRankType_D, 0},
	} {
		patterns = append(patterns, &papi.FanEventRankPattern{
			RankType: pattern.rank, ThresholdPoint: int32(min(int64(math.MaxInt32), pattern.threshold)),
		})
	}
	return patterns
}

func syncActiveFanEvent(ctx context.Context, suid string, progress *papi.FanEventProgress) error {
	info, err := opuser.FindUserFanEventInfo(ctx, suid)
	if err != nil {
		return err
	}
	viber.SyncActiveFanEventInfo(info, progress)
	return opuser.UpsertUserFanEventInfo(ctx, suid, info)
}

func fanEventScore(progress *papi.FanEventProgress) int64 {
	var score int64
	for _, character := range progress.GetCharacters() {
		score += int64(character.GetActivityPoint())
	}
	return score
}

func totalFanEventSteps(progress *papi.FanEventProgress) int32 {
	var total int64
	for _, character := range progress.GetCharacters() {
		total += int64(character.GetCompletedStep())
	}
	return int32(min(int64(math.MaxInt32), total))
}

func applyFanEventFanConversion(
	character *ptransaction.UserCharacter,
	coefficientMillionths int32,
) error {
	if character == nil || coefficientMillionths <= 0 {
		return nil
	}
	coefficient := int64(coefficientMillionths)
	// Core-fan growth is a maturation chain, not newly-created fans: a share
	// of light fans become middle fans while the same share of the existing
	// middle fans become heavy fans. Total fan count therefore stays constant.
	lightToMiddle := (int64(character.GetLightFanAmount())*coefficient + 500000) / 1000000
	middleToHeavy := (int64(character.GetMiddleFanAmount())*coefficient + 500000) / 1000000
	lightToMiddle = min(lightToMiddle, int64(character.GetLightFanAmount()))
	middleToHeavy = min(middleToHeavy, int64(character.GetMiddleFanAmount()))
	light := int64(character.GetLightFanAmount()) - lightToMiddle
	middle := int64(character.GetMiddleFanAmount()) + lightToMiddle - middleToHeavy
	heavy := int64(character.GetHeavyFanAmount()) + middleToHeavy
	if light < 0 || middle < 0 || heavy < 0 || light > math.MaxInt32 || middle > math.MaxInt32 || heavy > math.MaxInt32 {
		return status.Error(codes.FailedPrecondition, "core-fan conversion overflows")
	}
	character.LightFanAmount = int32(light)
	character.MiddleFanAmount = int32(middle)
	character.HeavyFanAmount = int32(heavy)
	return nil
}

func fanEventRank(patterns []*papi.FanEventRankPattern, score int64) (penum.ResultRankType, int32) {
	for _, pattern := range patterns {
		if score >= int64(pattern.GetThresholdPoint()) {
			return pattern.GetRankType(), pattern.GetPlus()
		}
	}
	return penum.ResultRankType_ResultRankType_D, 0
}

func fanEventRankKey(rank penum.ResultRankType) string {
	switch rank {
	case penum.ResultRankType_ResultRankType_C:
		return "C"
	case penum.ResultRankType_ResultRankType_B:
		return "B"
	case penum.ResultRankType_ResultRankType_A:
		return "A"
	case penum.ResultRankType_ResultRankType_S:
		return "S"
	case penum.ResultRankType_ResultRankType_Splus:
		return "Splus"
	default:
		return "D"
	}
}

func fanEventNextLevelLocked(fanEvent *papi.FanEvent, exp int32) bool {
	return fanEvent != nil && fanEvent.GetNextLevelRequiredExp() > 0 &&
		exp >= fanEvent.GetNextLevelRequiredExp() && !fanEvent.GetIsNextLevelUnlocked()
}

func advanceCharacterActivityLevel(ctx context.Context, character *ptransaction.UserCharacter) error {
	for {
		next, err := opmaster.FindCharacterActivityLevel(ctx, int64(character.GetActivityLevel()+1))
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil
			}
			return err
		}
		if character.GetActivityExp() < int64(next.GetRequiredExp()) {
			return nil
		}
		character.ActivityLevel = next.GetLevel()
	}
}

func updateFanEventRanking(ctx context.Context, suid string, progress *papi.FanEventProgress) error {
	ranking, err := opuser.FindUserFanEventRanking(ctx, suid, progress.GetActivityFanEventId())
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		ranking = &umodel.UserFanEventRanking{SUID: suid, ActivityFanEventId: progress.GetActivityFanEventId()}
	}
	score := fanEventScore(progress)
	if score <= ranking.BestScore {
		return nil
	}
	ranking.BestScore = score
	ranking.CharacterIds = ranking.CharacterIds[:0]
	for _, character := range progress.GetCharacters() {
		ranking.CharacterIds = append(ranking.CharacterIds, character.GetCharacterId())
	}
	return opuser.UpsertUserFanEventRanking(ctx, ranking)
}
