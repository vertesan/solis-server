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
	"google.golang.org/protobuf/proto"
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
			ctx, suid, req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds(), now,
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
		stepSeconds := setting.GetSecondsPerActivityFanEventStep()
		stepStamina := fanEvent.GetStepConsumptionStamina()
		if stepSeconds <= 0 || stepStamina <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "fan event timing is invalid")
		}
		updated := &papi.UpdatedData{}
		speedRate := 1.0
		specialCheerPoint := int32(1)
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
				specialCheerPoint += item.GetValue()
			}
		}
		coefficient := setting.GetActivityFanEventIntermediatePointCoefficientPermil()
		if coefficient <= 0 {
			coefficient = 1000
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
			RankPatterns:                       fanEventRankPatterns(maximumScore),
			ActivityPerformanceId:              fanEvent.GetActivityPerformanceId(),
			MaleNpcSpineId:                     "female-00",
			FemaleNpcSpineId:                   "male-00",
			SpecialFanCheerPoint:               specialCheerPoint,
			SpeedRate:                          speedRate,
		}
		if err := opuser.UpsertUserFanEventRun(ctx, suid, progress); err != nil {
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
		if _, err := reconcileFanEventProgress(ctx, suid, progress, time.Now().UnixMilli()); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserFanEventRun(ctx, suid, progress); err != nil {
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
	characterSteps, err := issueFanEventSteps(ctx, progress, req.GetSteps())
	if err != nil {
		return nil, err
	}
	return &papi.ActivityFetchFanEventStepsResponse{
		CharacterSteps: characterSteps,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
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
		updated := &papi.UpdatedData{}
		superModeWasActive := progress.GetUsedSuperModeItem()
		superEffectValue := int32(0)
		if superModeWasActive {
			superEffectValue, err = fanEventSuperModeEffectValue(ctx)
			if err != nil {
				return nil, err
			}
		}
		if req.GetUseItemId() != "" {
			if superModeWasActive {
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
			progress.UsedSuperModeItem = true
		}
		superMultiplier := int64(1000) + int64(superEffectValue)
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
		stepMillis := int64(float64(setting.GetSecondsPerActivityFanEventStep()*1000) / max(progress.GetSpeedRate(), 1))
		if stepMillis <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "fan event step duration is invalid")
		}
		for _, character := range progress.GetCharacters() {
			result, ok := resultByID[character.GetCharacterId()]
			if !ok {
				return nil, status.Error(codes.InvalidArgument, "fan event character result does not match active lineup")
			}
			userCharacter, err := opuser.FindUserCharacter(ctx, suid, character.GetCharacterId())
			if err != nil {
				return nil, err
			}
			basePoint, err := fanEventBasePoint(ctx, userCharacter, progress.GetStepActivityPointCoefficientPermil())
			if err != nil {
				return nil, err
			}
			allowedStep := fanEventAllowedStep(progress, character, now, stepMillis)
			beforeStep := character.GetCompletedStep()
			var reduction int64
			for _, stepResult := range result.GetStepResults() {
				if stepResult.GetStep() != character.GetCompletedStep()+1 || stepResult.GetStep() > allowedStep {
					return nil, status.Error(codes.InvalidArgument, "fan event steps must be sequential and elapsed")
				}
				maxPoint := int64(basePoint) * superMultiplier / 1000
				if stepResult.GetActivityPoint() <= 0 || int64(stepResult.GetActivityPoint()) > maxPoint {
					return nil, status.Error(codes.InvalidArgument, "fan event activity point is invalid")
				}
				maxReduction := int64(0)
				if superModeWasActive {
					maxReduction = min(stepMillis, int64(superEffectValue))
				}
				if stepResult.GetReductionMilliSeconds() < 0 || stepResult.GetReductionMilliSeconds() > maxReduction {
					return nil, status.Error(codes.InvalidArgument, "fan event time reduction is invalid")
				}
				character.CompletedStep++
				character.ActivityPoint += stepResult.GetActivityPoint()
				reduction += stepResult.GetReductionMilliSeconds()
			}
			delta := character.GetCompletedStep() - beforeStep
			character.ActivityStamina = max(int32(0), character.GetActivityStamina()-delta*progress.GetStepConsumptionStamina())
			character.FinishTime = max(progress.GetStartTime(), character.GetFinishTime()-reduction)
			character.LastSavedActivityPoint = character.GetActivityPoint()
			progress.CurrentCheerPoint += (character.GetCompletedStep()/10 - beforeStep/10) * progress.GetSpecialFanCheerPoint()
		}
		if req.GetExecCheer() {
			required := setting.GetActivityFanEventRequiredCheerPoint()
			if required <= 0 || progress.GetCurrentCheerPoint() < required {
				return nil, status.Error(codes.FailedPrecondition, "fan event cheer point is insufficient")
			}
			progress.CurrentCheerPoint -= required
			progress.NewSpecialFansCount++
		}
		if err := opuser.UpsertUserFanEventRun(ctx, suid, progress); err != nil {
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
		stepCursors := make([]int32, 0, len(progress.GetCharacters()))
		for _, character := range progress.GetCharacters() {
			stepCursors = append(stepCursors, character.GetCompletedStep())
		}
		issuedSteps, err := issueFanEventSteps(ctx, progress, stepCursors)
		if err != nil {
			return nil, err
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
		if _, err := reconcileFanEventProgress(ctx, suid, progress, now); err != nil {
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
		rewardIDs := []string{fanEvent.GetFixedRewardId(), fanEventRankRewardID(fanEvent, rankType)}
		rewards := make([]*ptransaction.Reward, 0)
		for _, rewardID := range rewardIDs {
			if rewardID == "" {
				continue
			}
			resolved, err := viber.ActivityRewardFromMaster(ctx, rewardID)
			if err != nil {
				return nil, err
			}
			for _, reward := range resolved {
				if err := viber.GrantActivityReward(ctx, suid, reward, updated); err != nil {
					return nil, err
				}
				rewards = mergeActivityReward(rewards, reward)
			}
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
			if character.GetActivityExp() <= math.MaxInt64-int64(activityCharacter.GetCompletedStep()) {
				character.ActivityExp += int64(activityCharacter.GetCompletedStep())
			}
			if err := advanceCharacterActivityLevel(ctx, character); err != nil {
				return nil, err
			}
			applyFanEventFanConversion(character, progress.GetActivityFanEventType(), progress.GetNewSpecialFansCount())
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
		before := &papi.ActivityLevelInfo{Exp: fanEvent.GetExp(), Level: fanEvent.GetLevel()}
		expGain := int32(min(
			int64(math.MaxInt32),
			int64(totalFanEventSteps(progress))*fanEventExperienceMultiplier(progress.GetActivityFanEventId()),
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
		_, masterLevel, err := findFanEventMaster(ctx, fanEvent.GetId(), fanEvent.GetLevel())
		if err != nil {
			return nil, err
		}
		maxLevel := masterLevel.GetLevel()
		masters, err := opmaster.FindAllActivityFanEvents(ctx)
		if err != nil {
			return nil, err
		}
		for _, master := range masters {
			if master.GetId() == fanEvent.GetId() {
				for _, level := range master.GetLevels() {
					maxLevel = max(maxLevel, level.GetLevel())
				}
			}
		}
		response = &papi.ActivityFinishFanEventResponse{
			ActivityPoint: int32(min(int64(math.MaxInt32), score)),
			RankType:      rankType,
			Plus:          rankPlus,
			Rewards:       rewards,
			RankPatterns:  progress.GetRankPatterns(),
			ActivityLevelProgress: &papi.ActivityLevelProgress{
				MaxActivityLevel: maxLevel,
				Before:           before,
				After:            &papi.ActivityLevelInfo{Exp: fanEvent.GetExp(), Level: fanEvent.GetLevel()},
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

func reconcileFanEventProgress(ctx context.Context, suid string, progress *papi.FanEventProgress, now int64) (bool, error) {
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return false, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return false, err
	}
	stepMillis := int64(float64(setting.GetSecondsPerActivityFanEventStep()*1000) / max(progress.GetSpeedRate(), 1))
	if stepMillis <= 0 {
		return false, status.Error(codes.FailedPrecondition, "fan event step duration is invalid")
	}
	changed := false
	for _, activityCharacter := range progress.GetCharacters() {
		allowed := fanEventAllowedStep(progress, activityCharacter, now, stepMillis)
		if allowed <= activityCharacter.GetCompletedStep() {
			continue
		}
		userCharacter, err := opuser.FindUserCharacter(ctx, suid, activityCharacter.GetCharacterId())
		if err != nil {
			return false, err
		}
		basePoint, err := fanEventBasePoint(ctx, userCharacter, progress.GetStepActivityPointCoefficientPermil())
		if err != nil {
			return false, err
		}
		delta := allowed - activityCharacter.GetCompletedStep()
		beforeStep := activityCharacter.GetCompletedStep()
		activityCharacter.CompletedStep = allowed
		activityCharacter.ActivityStamina = max(int32(0), activityCharacter.GetActivityStamina()-delta*progress.GetStepConsumptionStamina())
		activityCharacter.ActivityPoint = int32(min(int64(math.MaxInt32), int64(activityCharacter.GetActivityPoint())+int64(delta)*int64(basePoint)))
		progress.CurrentCheerPoint += (allowed/10 - beforeStep/10) * progress.GetSpecialFanCheerPoint()
		changed = true
	}
	return changed, nil
}

func fanEventAllowedStep(progress *papi.FanEventProgress, character *papi.FanEventCharacterInfo, now, stepMillis int64) int32 {
	originalFinishTime := progress.GetStartTime() + int64(character.GetMaxStep())*stepMillis
	timeReduction := max(int64(0), originalFinishTime-character.GetFinishTime())
	elapsed := max(int64(0), now-progress.GetStartTime())
	return int32(min(int64(character.GetMaxStep()), (elapsed+timeReduction)/stepMillis))
}

func fanEventSuperModeEffectValue(ctx context.Context) (int32, error) {
	item, err := opmaster.FindItem(ctx, fanEventSuperModeItemID)
	if err != nil {
		return 0, err
	}
	if item.GetType() != penum.ItemType_ItemType_ActivityFanEventSuperMode || item.GetValue() <= 0 {
		return 0, status.Error(codes.FailedPrecondition, "fan event super mode item master is invalid")
	}
	return item.GetValue(), nil
}

func issueFanEventSteps(
	ctx context.Context,
	progress *papi.FanEventProgress,
	cursors []int32,
) ([]*papi.FanEventCharacterSteps, error) {
	if len(cursors) != len(progress.GetCharacters()) {
		return nil, status.Error(codes.InvalidArgument, "one step cursor is required per character")
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return nil, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return nil, err
	}
	fetchSize := setting.GetActivityFanEventFetchStepsSize()
	if fetchSize <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "fan event fetch size is invalid")
	}
	superEffectValue := int32(0)
	if progress.GetUsedSuperModeItem() {
		superEffectValue, err = fanEventSuperModeEffectValue(ctx)
		if err != nil {
			return nil, err
		}
	}
	characterSteps := make([]*papi.FanEventCharacterSteps, 0, len(progress.GetCharacters()))
	for index, character := range progress.GetCharacters() {
		cursor := cursors[index]
		if cursor < 0 || cursor > character.GetMaxStep() {
			return nil, status.Error(codes.InvalidArgument, "fan event step cursor is out of range")
		}
		lastStep := min(character.GetMaxStep(), cursor+fetchSize)
		steps := make([]*papi.FanEventStep, 0, max(int32(0), lastStep-cursor))
		for step := cursor + 1; step <= lastStep; step++ {
			issued := &papi.FanEventStep{
				Step:        step,
				Happenings:  []*papi.FanEventHappening{},
				ItemEffects: []*papi.FanEventItemEffect{},
			}
			if superEffectValue > 0 {
				issued.ItemEffects = append(issued.ItemEffects, &papi.FanEventItemEffect{
					Type:  penum.ItemType_ItemType_ActivityFanEventSuperMode,
					Value: superEffectValue,
				})
			}
			steps = append(steps, issued)
		}
		characterSteps = append(characterSteps, &papi.FanEventCharacterSteps{
			CharacterId: character.GetCharacterId(),
			Steps:       steps,
		})
	}
	return characterSteps, nil
}

func syncActiveFanEvent(ctx context.Context, suid string, progress *papi.FanEventProgress) error {
	info, err := opuser.FindUserFanEventInfo(ctx, suid)
	if err != nil {
		return err
	}
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

func fanEventExperienceMultiplier(activityFanEventID string) int64 {
	switch activityFanEventID {
	case "act-fan-02":
		return 2
	case "act-fan-03":
		return 3
	default:
		return 1
	}
}

func applyFanEventFanConversion(
	character *ptransaction.UserCharacter,
	fanEventType penum.ActivityFanEventType,
	newSpecialFans int32,
) {
	if character == nil || newSpecialFans <= 0 || fanEventType == penum.ActivityFanEventType_ActivityFanEventType_Handshake {
		return
	}
	converted := min(character.GetLightFanAmount(), newSpecialFans)
	if converted <= 0 {
		return
	}
	coreFans := converted * 2 / 3
	otherFans := converted - coreFans
	character.LightFanAmount -= converted
	switch fanEventType {
	case penum.ActivityFanEventType_ActivityFanEventType_Sign:
		character.MiddleFanAmount += coreFans
		character.HeavyFanAmount += otherFans
	case penum.ActivityFanEventType_ActivityFanEventType_PhotoSession:
		character.HeavyFanAmount += coreFans
		character.MiddleFanAmount += otherFans
	}
}

func fanEventRank(patterns []*papi.FanEventRankPattern, score int64) (penum.ResultRankType, int32) {
	for _, pattern := range patterns {
		if score >= int64(pattern.GetThresholdPoint()) {
			return pattern.GetRankType(), pattern.GetPlus()
		}
	}
	return penum.ResultRankType_ResultRankType_D, 0
}

func fanEventRankRewardID(fanEvent *papi.FanEvent, rank penum.ResultRankType) string {
	switch rank {
	case penum.ResultRankType_ResultRankType_Splus, penum.ResultRankType_ResultRankType_S:
		return fanEvent.GetRankSRewardId()
	case penum.ResultRankType_ResultRankType_A:
		return fanEvent.GetRankARewardId()
	case penum.ResultRankType_ResultRankType_B:
		return fanEvent.GetRankBRewardId()
	case penum.ResultRankType_ResultRankType_C:
		return fanEvent.GetRankCRewardId()
	default:
		return fanEvent.GetRankDRewardId()
	}
}

func mergeActivityReward(rewards []*ptransaction.Reward, reward *ptransaction.Reward) []*ptransaction.Reward {
	for _, current := range rewards {
		if current.GetResourceType() == reward.GetResourceType() && current.GetResourceId() == reward.GetResourceId() {
			current.Amount += reward.GetAmount()
			return rewards
		}
	}
	return append(rewards, proto.Clone(reward).(*ptransaction.Reward))
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
