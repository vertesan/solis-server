package activity

import (
	"context"
	"errors"
	"math"
	"time"

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
		if err := reconcileFanEventInfo(ctx, suid, now, info); err != nil {
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
		characters, lineup, err := validateActivityLineup(
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
		balanceProfile, err := findFanEventBalanceProfile(fanEvent.GetId(), fanEvent.GetLevel())
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

// reconcileFanEventInfo refreshes the condition-backed fan-event gates before
// a direct Start call, which need not be preceded by Home.Enter.
func reconcileFanEventInfo(ctx context.Context, suid string, now int64, info *papi.FanEventInfo) error {
	return viber.ReconcileFanEventInfo(ctx, suid, now, viber.NewCommerceConditionEvaluator(ctx, suid, now), info)
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
