package viber

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	storyMessageDelayMinutes   = 60
	instantMessageDelayMinutes = 12 * 60
	telephoneDelayMinutes      = 18 * 60
)

type narrativeUnlockKind uint8

const (
	NarrativeUnlockAll narrativeUnlockKind = iota
	NarrativeUnlockReadStory
	NarrativeUnlockFinishedMessage
	NarrativeUnlockNone
)

func ReconcileNarrativeSchedules(
	ctx context.Context,
	suid string,
	now int64,
	updated *papi.UpdatedData,
	unlockKind narrativeUnlockKind,
	unlockID string,
) error {
	if updated == nil {
		return nil
	}
	feature := ""
	state, err := opuser.FindUserGodState(ctx, suid)
	if err == nil {
		feature = state.MessageFeature
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	switch feature {
	case "", umodel.GodMessageFeatureNone, umodel.GodMessageFeatureAll, umodel.GodMessageFeatureEnjoy:
	default:
		return errors.New("user has an invalid god message feature")
	}
	if feature == umodel.GodMessageFeatureAll {
		return nil
	}

	masters, err := opmaster.FindAllMessages(ctx)
	if err != nil {
		return err
	}
	masterTelephones, err := opmaster.FindAllTelephones(ctx)
	if err != nil {
		return err
	}
	masterConditions, err := opmaster.FindAllConditions(ctx)
	if err != nil {
		return err
	}
	conditionsByID := make(map[string]*pmaster.Condition, len(masterConditions))
	for _, condition := range masterConditions {
		conditionsByID[condition.GetId()] = condition
	}
	userMessages, err := opuser.FindAllUserMessages(ctx, suid)
	if err != nil {
		return err
	}
	schedules, err := opuser.FindAllUserMessageSchedules(ctx, suid)
	if err != nil {
		return err
	}
	userTelephones, err := opuser.FindAllUserTelephones(ctx, suid)
	if err != nil {
		return err
	}

	evaluator := NewCommerceConditionEvaluator(ctx, suid, now)
	evaluator.primeConditions(masterConditions)
	messagesByID := make(map[string]*ptransaction.UserMessage, len(userMessages)+len(masters))
	for _, message := range userMessages {
		messagesByID[message.GetMessageId()] = message
	}
	schedulesByID := make(map[string]*ptransaction.UserMessageSchedule, len(schedules)+len(masters))
	for _, schedule := range schedules {
		schedulesByID[schedule.GetMessageId()] = schedule
	}
	changedMessages := make(map[string]struct{})
	changedSchedules := make(map[string]struct{})

	for _, master := range masters {
		if messagesByID[master.GetId()] != nil || !narrativeConditionEligible(
			feature, master.GetUnlockConditionId(), unlockKind, unlockID, conditionsByID,
		) {
			continue
		}
		satisfied := feature == umodel.GodMessageFeatureEnjoy
		if !satisfied {
			satisfied, err = evaluator.Satisfied(master.GetUnlockConditionId())
			if err != nil {
				return err
			}
		}
		if !satisfied {
			continue
		}
		message := &ptransaction.UserMessage{
			MessageId:         master.GetId(),
			MessageStatusType: penum.MessageStatusType_MessageStatusType_Unlocked,
		}
		messagesByID[master.GetId()] = message
		changedMessages[master.GetId()] = struct{}{}
	}

	for _, master := range masters {
		message := messagesByID[master.GetId()]
		if message == nil || message.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Unlocked {
			continue
		}
		if schedulesByID[master.GetId()] == nil {
			schedulesByID[master.GetId()] = &ptransaction.UserMessageSchedule{MessageId: master.GetId()}
			changedSchedules[master.GetId()] = struct{}{}
		}
	}

	activeMessageGroups := make(map[string]struct{})
	mastersByID := make(map[string]*pmaster.Message, len(masters))
	for _, master := range masters {
		mastersByID[master.GetId()] = master
	}
	for _, message := range messagesByID {
		if message.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Scheduled &&
			message.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Received {
			continue
		}
		if master := mastersByID[message.GetMessageId()]; master != nil {
			activeMessageGroups[narrativeQueueKey(master.GetMessageGroupId(), master.GetCharacterId())] = struct{}{}
		}
	}
	for _, master := range masters {
		message := messagesByID[master.GetId()]
		if message == nil || message.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Unlocked {
			continue
		}
		key := narrativeQueueKey(master.GetMessageGroupId(), master.GetCharacterId())
		if _, active := activeMessageGroups[key]; active {
			continue
		}
		delayLimit := instantMessageDelayMinutes
		if conditionReadsStory(master.GetUnlockConditionId(), conditionsByID, make(map[string]struct{})) {
			delayLimit = storyMessageDelayMinutes
		}
		delay, err := randomNarrativeDelay(delayLimit)
		if err != nil {
			return err
		}
		schedule := schedulesByID[master.GetId()]
		schedule.ScheduledTime = now + delay
		message.MessageStatusType = penum.MessageStatusType_MessageStatusType_Scheduled
		if delay == 0 {
			message.MessageStatusType = penum.MessageStatusType_MessageStatusType_Received
			message.ArrivedTime = now
		}
		changedMessages[master.GetId()] = struct{}{}
		changedSchedules[master.GetId()] = struct{}{}
		activeMessageGroups[key] = struct{}{}
	}

	for _, master := range masters {
		if _, changed := changedMessages[master.GetId()]; !changed {
			continue
		}
		message := messagesByID[master.GetId()]
		if err := opuser.UpsertUserMessage(ctx, suid, message); err != nil {
			return err
		}
		putUpdatedMessage(updated, message)
	}
	for _, master := range masters {
		if _, changed := changedSchedules[master.GetId()]; !changed {
			continue
		}
		schedule := schedulesByID[master.GetId()]
		if err := opuser.UpsertUserMessageSchedule(ctx, suid, schedule); err != nil {
			return err
		}
		putUpdatedMessageSchedule(updated, schedule)
	}

	embeddedTelephoneIDs := make(map[string]struct{})
	for _, master := range masters {
		for _, detail := range master.GetDetails() {
			if detail.GetTelephoneId() != "" {
				embeddedTelephoneIDs[detail.GetTelephoneId()] = struct{}{}
			}
		}
	}
	telephonesByID := make(map[string]*ptransaction.UserTelephone, len(userTelephones)+len(masterTelephones))
	for _, telephone := range userTelephones {
		telephonesByID[telephone.GetTelephoneId()] = telephone
	}
	changedTelephones := make(map[string]struct{})
	for _, master := range masterTelephones {
		if telephonesByID[master.GetId()] != nil {
			continue
		}
		if _, embedded := embeddedTelephoneIDs[master.GetId()]; embedded {
			continue
		}
		if !narrativeConditionEligible(
			feature, master.GetUnlockConditionId(), unlockKind, unlockID, conditionsByID,
		) {
			continue
		}
		satisfied := feature == umodel.GodMessageFeatureEnjoy
		if !satisfied {
			satisfied, err = evaluator.Satisfied(master.GetUnlockConditionId())
			if err != nil {
				return err
			}
		}
		if !satisfied {
			continue
		}
		telephone := &ptransaction.UserTelephone{
			TelephoneId:         master.GetId(),
			UnlockedTime:        now,
			TelephoneStatusType: penum.TelephoneStatusType_TelephoneStatusType_Unlocked,
		}
		telephonesByID[master.GetId()] = telephone
		changedTelephones[master.GetId()] = struct{}{}
	}

	telephoneMastersByID := make(map[string]*pmaster.Telephone, len(masterTelephones))
	activeTelephoneGroups := make(map[string]struct{})
	for _, master := range masterTelephones {
		telephoneMastersByID[master.GetId()] = master
	}
	for _, telephone := range telephonesByID {
		if telephone.GetTelephoneStatusType() != penum.TelephoneStatusType_TelephoneStatusType_Scheduled &&
			telephone.GetTelephoneStatusType() != penum.TelephoneStatusType_TelephoneStatusType_Received {
			continue
		}
		if master := telephoneMastersByID[telephone.GetTelephoneId()]; master != nil {
			activeTelephoneGroups[narrativeQueueKey(master.GetMessageGroupId(), master.GetCharacterId())] = struct{}{}
		}
	}
	for _, master := range masterTelephones {
		telephone := telephonesByID[master.GetId()]
		if telephone == nil || telephone.GetTelephoneStatusType() != penum.TelephoneStatusType_TelephoneStatusType_Unlocked {
			continue
		}
		if _, embedded := embeddedTelephoneIDs[master.GetId()]; embedded {
			continue
		}
		key := narrativeQueueKey(master.GetMessageGroupId(), master.GetCharacterId())
		if _, active := activeTelephoneGroups[key]; active {
			continue
		}
		delay, err := randomNarrativeDelay(telephoneDelayMinutes)
		if err != nil {
			return err
		}
		telephone.ScheduledTime = now + delay
		telephone.TelephoneStatusType = penum.TelephoneStatusType_TelephoneStatusType_Scheduled
		if delay == 0 {
			telephone.TelephoneStatusType = penum.TelephoneStatusType_TelephoneStatusType_Received
		}
		changedTelephones[master.GetId()] = struct{}{}
		activeTelephoneGroups[key] = struct{}{}
	}
	for _, master := range masterTelephones {
		if _, changed := changedTelephones[master.GetId()]; !changed {
			continue
		}
		telephone := telephonesByID[master.GetId()]
		if err := opuser.UpsertUserTelephone(ctx, suid, telephone); err != nil {
			return err
		}
		putUpdatedTelephone(updated, telephone)
	}
	return nil
}

func narrativeConditionEligible(
	feature string,
	conditionID string,
	unlockKind narrativeUnlockKind,
	unlockID string,
	conditions map[string]*pmaster.Condition,
) bool {
	if feature == umodel.GodMessageFeatureNone || unlockKind == NarrativeUnlockNone {
		return false
	}
	if feature == umodel.GodMessageFeatureEnjoy {
		return unlockKind == NarrativeUnlockAll && conditionID == ""
	}
	// Unconditional entries are introduced by their owning game feature. The
	// reconciler handles dependency-based unlocks for ordinary users.
	if conditionID == "" {
		return false
	}
	switch unlockKind {
	case NarrativeUnlockAll:
		return true
	case NarrativeUnlockReadStory:
		return conditionMatchesNarrativeDependency(
			conditionID, conditions, unlockKind, unlockID, make(map[string]struct{}),
		)
	case NarrativeUnlockFinishedMessage:
		return conditionMatchesNarrativeDependency(
			conditionID, conditions, unlockKind, unlockID, make(map[string]struct{}),
		)
	default:
		return false
	}
}

func narrativeQueueKey(groupID, characterID string) string {
	if groupID != "" {
		return "group:" + groupID
	}
	return characterID
}

func randomNarrativeDelay(maxMinutes int) (int64, error) {
	index, err := CryptoRandomIndex(maxMinutes + 1)
	if err != nil {
		return 0, err
	}
	return int64(index) * time.Minute.Milliseconds(), nil
}

func conditionReadsStory(
	conditionID string,
	conditions map[string]*pmaster.Condition,
	visited map[string]struct{},
) bool {
	if conditionID == "" {
		return false
	}
	if _, seen := visited[conditionID]; seen {
		return false
	}
	visited[conditionID] = struct{}{}
	condition := conditions[conditionID]
	if condition == nil {
		return false
	}
	for _, setting := range condition.GetSettings() {
		if setting.GetReadStory().GetStoryId() != "" {
			return true
		}
		if dependency := setting.GetSatisfyCondition(); dependency != nil &&
			conditionReadsStory(dependency.GetConditionId(), conditions, visited) {
			return true
		}
	}
	return false
}

func conditionMatchesNarrativeDependency(
	conditionID string,
	conditions map[string]*pmaster.Condition,
	kind narrativeUnlockKind,
	targetID string,
	visited map[string]struct{},
) bool {
	if conditionID == "" || targetID == "" {
		return false
	}
	if _, seen := visited[conditionID]; seen {
		return false
	}
	visited[conditionID] = struct{}{}
	condition := conditions[conditionID]
	if condition == nil {
		return false
	}
	for _, setting := range condition.GetSettings() {
		if kind == NarrativeUnlockReadStory && setting.GetReadStory().GetStoryId() == targetID {
			return true
		}
		if kind == NarrativeUnlockFinishedMessage && setting.GetFinishedMessage().GetMessageId() == targetID {
			return true
		}
		if dependency := setting.GetSatisfyCondition(); dependency != nil &&
			conditionMatchesNarrativeDependency(dependency.GetConditionId(), conditions, kind, targetID, visited) {
			return true
		}
	}
	return false
}

func putUpdatedMessage(updated *papi.UpdatedData, message *ptransaction.UserMessage) {
	for index, current := range updated.Messages {
		if current.GetMessageId() == message.GetMessageId() {
			updated.Messages[index] = message
			return
		}
	}
	updated.Messages = append(updated.Messages, message)
}

func putUpdatedMessageSchedule(updated *papi.UpdatedData, schedule *ptransaction.UserMessageSchedule) {
	for index, current := range updated.MessageSchedules {
		if current.GetMessageId() == schedule.GetMessageId() {
			updated.MessageSchedules[index] = schedule
			return
		}
	}
	updated.MessageSchedules = append(updated.MessageSchedules, schedule)
}

func putUpdatedTelephone(updated *papi.UpdatedData, telephone *ptransaction.UserTelephone) {
	for index, current := range updated.Telephones {
		if current.GetTelephoneId() == telephone.GetTelephoneId() {
			updated.Telephones[index] = telephone
			return
		}
	}
	updated.Telephones = append(updated.Telephones, telephone)
}
