package servers

import (
	"context"
	"errors"
	"hash/fnv"
	"math"
	"sort"
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

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const messageTimelinePageSize = 30

type MessageServer struct {
	papi.UnimplementedMessageServer
}

type messageTimelineState struct {
	infos     []*papi.TimelineInfo
	usersById map[string]*ptransaction.UserMessage
	masters   map[string]*pmaster.Message
}

func (s *MessageServer) ListGroup(ctx context.Context, _ *papi.Empty) (*papi.MessageListGroupResponse, error) {
	state, err := loadMessageTimelineState(ctx, viber.GetSuid(ctx).String())
	if err != nil {
		return nil, err
	}
	latestByGroup := make(map[string]*papi.TimelineInfo)
	for _, info := range state.infos {
		if latest := latestByGroup[info.MessageGroupId]; latest == nil || info.TimelineId > latest.TimelineId {
			latestByGroup[info.MessageGroupId] = info
		}
	}
	groupIds := make([]string, 0, len(latestByGroup))
	for id := range latestByGroup {
		groupIds = append(groupIds, id)
	}
	sort.Strings(groupIds)
	infos := make([]*papi.MessageGroupInfo, 0, len(groupIds))
	for _, id := range groupIds {
		infos = append(infos, &papi.MessageGroupInfo{MessageGroupId: id, LatestTimelineInfo: latestByGroup[id]})
	}
	return &papi.MessageListGroupResponse{Infos: infos, CommonResponse: viber.NewCommonResponse(nil)}, nil
}

func (s *MessageServer) Timeline(ctx context.Context, req *papi.MessageTimelineRequest) (*papi.MessageTimelineResponse, error) {
	if req.GetMessageGroupId() == "" || req.GetTimelineId() < 0 {
		return nil, viber.InvalidArgumentf("messageGroupId and a non-negative timelineId are required")
	}
	state, err := loadMessageTimelineState(ctx, viber.GetSuid(ctx).String())
	if err != nil {
		return nil, err
	}
	infos := make([]*papi.TimelineInfo, 0)
	for _, info := range state.infos {
		if info.MessageGroupId != req.MessageGroupId || req.TimelineId > 0 && info.TimelineId >= req.TimelineId {
			continue
		}
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].TimelineId > infos[j].TimelineId })
	hasNext := len(infos) > messageTimelinePageSize
	if hasNext {
		infos = infos[:messageTimelinePageSize]
	}
	return &papi.MessageTimelineResponse{
		Infos: infos, HasNext: hasNext, CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *MessageServer) Receive(ctx context.Context, req *papi.MessageReceiveRequest) (*papi.MessageReceiveResponse, error) {
	if len(req.GetMessageIds()) == 0 {
		return nil, viber.InvalidArgumentf("at least one messageId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.MessageReceiveResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
		updated := &papi.UpdatedData{}
		seen := make(map[string]struct{}, len(req.GetMessageIds()))
		for _, messageID := range req.GetMessageIds() {
			if messageID == "" {
				return nil, viber.InvalidArgumentf("messageId must not be empty")
			}
			if _, duplicate := seen[messageID]; duplicate {
				continue
			}
			seen[messageID] = struct{}{}
			if _, err := opmaster.FindMessage(ctx, messageID); err != nil {
				return nil, err
			}
			message, err := opuser.FindUserMessage(ctx, suid, messageID)
			if err != nil {
				return nil, err
			}
			switch message.GetMessageStatusType() {
			case penum.MessageStatusType_MessageStatusType_Received,
				penum.MessageStatusType_MessageStatusType_Finished:
				continue
			case penum.MessageStatusType_MessageStatusType_Scheduled:
			default:
				return nil, status.Errorf(codes.FailedPrecondition, "message %q is not scheduled", messageID)
			}
			schedule, err := opuser.FindUserMessageSchedule(ctx, suid, messageID)
			if err != nil {
				return nil, err
			}
			if schedule.GetScheduledTime() <= 0 || schedule.GetScheduledTime() > now {
				return nil, status.Errorf(codes.FailedPrecondition, "message %q has not arrived", messageID)
			}
			message.MessageStatusType = penum.MessageStatusType_MessageStatusType_Received
			message.ArrivedTime = schedule.GetScheduledTime()
			if err := opuser.UpsertUserMessage(ctx, suid, message); err != nil {
				return nil, err
			}
			updated.Messages = append(updated.Messages, message)
			updated.MessageSchedules = append(updated.MessageSchedules, schedule)
		}
		response = &papi.MessageReceiveResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *MessageServer) Save(ctx context.Context, req *papi.MessageSaveRequest) (*papi.MessageSaveResponse, error) {
	if req.GetMessageGroupId() == "" || req.GetTimelineId() <= 0 || req.GetMessageDetailId() == "" {
		return nil, viber.InvalidArgumentf("messageGroupId, a positive timelineId, and messageDetailId are required")
	}
	suid := viber.GetSuid(ctx).String()
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		state, err := loadMessageTimelineState(ctx, suid)
		if err != nil {
			return nil, err
		}
		info, userMessage, master := findTimelineMessage(state, req.MessageGroupId, req.TimelineId)
		if info == nil {
			return nil, status.Error(codes.NotFound, "message timeline was not found")
		}
		if userMessage.GetMessageStatusType() == penum.MessageStatusType_MessageStatusType_Finished {
			return nil, status.Error(codes.FailedPrecondition, "finished message progress cannot be changed")
		}
		if userMessage.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Received {
			return nil, status.Error(codes.FailedPrecondition, "message has not been received")
		}
		path, selections, err := resolveMessageDetails(master, req.SelectMessageDetailIds, req.MessageDetailId)
		if err != nil {
			return nil, err
		}
		if err := validateMessageProgressExtension(master, info, userMessage, path); err != nil {
			return nil, err
		}
		userMessage.MessageStatusType = penum.MessageStatusType_MessageStatusType_Received
		userMessage.SelectMessageDetailIds = selections
		if err := opuser.UpsertUserMessage(ctx, suid, userMessage); err != nil {
			return nil, err
		}
		return nil, opuser.UpsertUserMessageProgress(ctx, &umodel.UserMessageProgress{
			SUID: suid, MessageId: userMessage.MessageId,
			LastMessageDetailId: req.MessageDetailId, LastMessageDetailTime: time.Now().UnixMilli(),
		})
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.MessageSaveResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}

func (s *MessageServer) Finish(ctx context.Context, req *papi.MessageFinishRequest) (*papi.MessageFinishResponse, error) {
	if req.GetMessageGroupId() == "" && req.GetTimelineId() == 0 {
		return &papi.MessageFinishResponse{}, nil
	}
	if req.GetMessageGroupId() == "" || req.GetTimelineId() <= 0 {
		return nil, viber.InvalidArgumentf("messageGroupId and a positive timelineId are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.MessageFinishResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		finishedTime := time.Now().UnixMilli()
		state, err := loadMessageTimelineState(ctx, suid)
		if err != nil {
			return nil, err
		}
		info, userMessage, master := findTimelineMessage(state, req.MessageGroupId, req.TimelineId)
		if userMessage == nil {
			return nil, status.Error(codes.NotFound, "message timeline was not found")
		}
		path, selections, err := resolveMessageDetails(master, req.SelectMessageDetailIds, "")
		if err != nil {
			return nil, err
		}
		terminal := path[len(path)-1]
		if userMessage.GetMessageStatusType() == penum.MessageStatusType_MessageStatusType_Finished {
			response = &papi.MessageFinishResponse{CommonResponse: viber.NewCommonResponse(nil)}
			return nil, nil
		}
		if err := validateMessageProgressExtension(master, info, userMessage, path); err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		isReliabilityUp := false
		userMessage.MessageStatusType = penum.MessageStatusType_MessageStatusType_Finished
		userMessage.SelectMessageDetailIds = selections
		if err := opuser.UpsertUserMessage(ctx, suid, userMessage); err != nil {
			return nil, err
		}
		updated.Messages = append(updated.Messages, userMessage)

		reliability := terminal.GetReliability()
		characterId := terminal.GetCharacterId()
		if characterId == "" {
			characterId = master.GetCharacterId()
		}
		if reliability < 0 || reliability > 0 && characterId == "" {
			return nil, status.Error(codes.FailedPrecondition, "message terminal has invalid reliability data")
		}
		if reliability > 0 && characterId != "" {
			character, err := opuser.FindUserCharacter(ctx, suid, characterId)
			if err != nil {
				return nil, err
			}
			if reliability > math.MaxInt32-character.Reliability {
				return nil, status.Error(codes.FailedPrecondition, "character reliability overflows")
			}
			character.Reliability += reliability
			if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
				return nil, err
			}
			updated.Characters = append(updated.Characters, character)
			missions, err := viber.AdvanceUserMissions(
				ctx, suid, penum.MissionType_MissionType_ReliabilityAchievement, int64(reliability), characterId,
			)
			if err != nil {
				return nil, err
			}
			updated.Missions = append(updated.Missions, missions...)
			isReliabilityUp = true
		}

		for _, detail := range path {
			telephoneID := detail.GetTelephoneId()
			if telephoneID == "" {
				continue
			}
			if _, err := opmaster.FindTelephone(ctx, telephoneID); err != nil {
				return nil, err
			}
			telephone, err := opuser.FindUserTelephone(ctx, suid, telephoneID)
			if errors.Is(err, mongo.ErrNoDocuments) {
				telephone = &ptransaction.UserTelephone{TelephoneId: telephoneID}
			} else if err != nil {
				return nil, err
			}
			if telephone.GetTelephoneStatusType() == penum.TelephoneStatusType_TelephoneStatusType_Finished {
				continue
			}
			if telephone.UnlockedTime == 0 {
				telephone.UnlockedTime = finishedTime
			}
			if telephone.ScheduledTime == 0 {
				telephone.ScheduledTime = telephone.UnlockedTime
			}
			telephone.TelephoneStatusType = penum.TelephoneStatusType_TelephoneStatusType_Finished
			if err := opuser.UpsertUserTelephone(ctx, suid, telephone); err != nil {
				return nil, err
			}
			updated.Telephones = append(updated.Telephones, telephone)
		}

		if err := opuser.UpsertUserMessageProgress(ctx, &umodel.UserMessageProgress{
			SUID: suid, MessageId: userMessage.MessageId,
			LastMessageDetailId: terminal.GetMessageDetailId(), LastMessageDetailTime: finishedTime,
		}); err != nil {
			return nil, err
		}

		common := viber.NewCommonResponse(updated)
		schedule, err := opuser.FindUserMessageSchedule(ctx, suid, userMessage.MessageId)
		if err == nil {
			if err := opuser.DeleteUserMessageSchedule(ctx, suid, userMessage.MessageId); err != nil {
				return nil, err
			}
			common.DeletedData.MessageSchedules = append(common.DeletedData.MessageSchedules, &ptransaction.UserMessageSchedule{
				MessageId: schedule.GetMessageId(),
			})
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		if err := viber.ReconcileNarrativeSchedulesAfterMessage(
			ctx, suid, finishedTime, userMessage.GetMessageId(), updated,
		); err != nil {
			return nil, err
		}
		response = &papi.MessageFinishResponse{IsReliabilityUp: isReliabilityUp, CommonResponse: common}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *MessageServer) SaveHistory(ctx context.Context, req *papi.MessageSaveHistoryRequest) (*papi.MessageSaveHistoryResponse, error) {
	if req.GetMessageId() == "" {
		return nil, viber.InvalidArgumentf("messageId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.MessageSaveHistoryResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		master, err := opmaster.FindMessage(ctx, req.MessageId)
		if err != nil {
			return nil, err
		}
		message, err := opuser.FindUserMessage(ctx, suid, req.MessageId)
		if err != nil {
			return nil, err
		}
		if message.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Finished {
			return nil, status.Error(codes.FailedPrecondition, "message history is unavailable before the message is finished")
		}
		_, selections, err := resolveMessagePath(master, req.SelectMessageDetailIds, "")
		if err != nil {
			return nil, err
		}
		message.SelectMessageDetailIds = mergeSortedStrings(message.SelectMessageDetailIds, selections)
		if err := opuser.UpsertUserMessage(ctx, suid, message); err != nil {
			return nil, err
		}
		response = &papi.MessageSaveHistoryResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			Messages: []*ptransaction.UserMessage{message},
		})}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func loadMessageTimelineState(ctx context.Context, suid string) (*messageTimelineState, error) {
	now := time.Now().UnixMilli()
	userMessages, err := opuser.FindAllUserMessages(ctx, suid)
	if err != nil {
		return nil, err
	}
	masterMessages, err := opmaster.FindAllMessages(ctx)
	if err != nil {
		return nil, err
	}
	progresses, err := opuser.FindAllUserMessageProgresses(ctx, suid)
	if err != nil {
		return nil, err
	}
	masters := make(map[string]*pmaster.Message, len(masterMessages))
	for _, master := range masterMessages {
		masters[master.Id] = master
	}
	progressByMessage := make(map[string]*umodel.UserMessageProgress, len(progresses))
	for _, progress := range progresses {
		progressByMessage[progress.MessageId] = progress
	}
	state := &messageTimelineState{
		infos:     make([]*papi.TimelineInfo, 0, len(userMessages)),
		usersById: make(map[string]*ptransaction.UserMessage, len(userMessages)),
		masters:   masters,
	}
	sort.Slice(userMessages, func(i, j int) bool { return userMessages[i].MessageId < userMessages[j].MessageId })
	usedTimelineIds := make(map[int64]struct{}, len(userMessages))
	for _, message := range userMessages {
		master := masters[message.MessageId]
		if master == nil || message.MessageStatusType < penum.MessageStatusType_MessageStatusType_Received ||
			message.GetArrivedTime() > now {
			continue
		}
		state.usersById[message.MessageId] = message
		timelineId := uniqueMessageTimelineId(message, usedTimelineIds)
		usedTimelineIds[timelineId] = struct{}{}
		info := &papi.TimelineInfo{
			MessageGroupId:         master.MessageGroupId,
			TimelineId:             timelineId,
			MessageId:              message.MessageId,
			ArrivedTime:            message.ArrivedTime,
			SelectMessageDetailIds: append([]string(nil), message.SelectMessageDetailIds...),
			Finished:               message.MessageStatusType == penum.MessageStatusType_MessageStatusType_Finished,
		}
		if progress := progressByMessage[message.MessageId]; progress != nil {
			info.LastMessageDetailId = progress.LastMessageDetailId
			info.LastMessageDetailTime = progress.LastMessageDetailTime
		} else if info.Finished {
			info.LastMessageDetailId = terminalMessageDetailId(master)
			info.LastMessageDetailTime = message.ArrivedTime
		}
		state.infos = append(state.infos, info)
	}
	return state, nil
}

func findTimelineMessage(
	state *messageTimelineState,
	groupId string,
	timelineId int64,
) (*papi.TimelineInfo, *ptransaction.UserMessage, *pmaster.Message) {
	for _, info := range state.infos {
		if info.MessageGroupId == groupId && info.TimelineId == timelineId {
			return info, state.usersById[info.MessageId], state.masters[info.MessageId]
		}
	}
	return nil, nil, nil
}

func messageTimelineId(message *ptransaction.UserMessage) int64 {
	base := message.ArrivedTime * 1_000_000
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(message.MessageId))
	return base + int64(hash.Sum32()%1_000_000)
}

func uniqueMessageTimelineId(message *ptransaction.UserMessage, used map[int64]struct{}) int64 {
	base := message.ArrivedTime * 1_000_000
	candidate := messageTimelineId(message)
	for {
		if _, exists := used[candidate]; !exists {
			return candidate
		}
		suffix := (candidate - base + 1) % 1_000_000
		candidate = base + suffix
	}
}

func resolveMessagePath(
	master *pmaster.Message,
	selectionIds []string,
	stopId string,
) (*pmaster.MessageDetail, []string, error) {
	path, selections, err := resolveMessageDetails(master, selectionIds, stopId)
	if err != nil {
		return nil, nil, err
	}
	return path[len(path)-1], selections, nil
}

func resolveMessageDetails(
	master *pmaster.Message,
	selectionIds []string,
	stopId string,
) ([]*pmaster.MessageDetail, []string, error) {
	if master == nil || len(master.GetDetails()) == 0 {
		return nil, nil, status.Error(codes.FailedPrecondition, "message has no detail graph")
	}
	details := make(map[string]*pmaster.MessageDetail, len(master.GetDetails()))
	indegree := make(map[string]int, len(master.GetDetails()))
	for _, detail := range master.GetDetails() {
		if detail.GetMessageDetailId() == "" || details[detail.GetMessageDetailId()] != nil {
			return nil, nil, status.Error(codes.FailedPrecondition, "message detail graph has an invalid identity")
		}
		details[detail.GetMessageDetailId()] = detail
		indegree[detail.GetMessageDetailId()] = 0
	}
	for _, detail := range master.GetDetails() {
		for _, nextId := range detail.GetNextMessageDetailIds() {
			if details[nextId] == nil {
				return nil, nil, status.Errorf(codes.FailedPrecondition, "message detail %q references missing detail %q", detail.GetMessageDetailId(), nextId)
			}
			indegree[nextId]++
		}
	}
	var root *pmaster.MessageDetail
	for id, degree := range indegree {
		if degree != 0 {
			continue
		}
		if root != nil {
			return nil, nil, status.Error(codes.FailedPrecondition, "message detail graph has multiple roots")
		}
		root = details[id]
	}
	if root == nil {
		return nil, nil, status.Error(codes.FailedPrecondition, "message detail graph has no root")
	}

	selected := make(map[string]struct{}, len(selectionIds))
	for _, id := range selectionIds {
		detail := details[id]
		if detail == nil || detail.GetChoiceText() == "" {
			return nil, nil, viber.InvalidArgumentf("message selection %q is not a choice in this message", id)
		}
		if _, duplicate := selected[id]; duplicate {
			return nil, nil, viber.InvalidArgumentf("message selection %q is duplicated", id)
		}
		selected[id] = struct{}{}
	}

	current := root
	path := make([]*pmaster.MessageDetail, 0, len(details))
	visited := make(map[string]struct{}, len(details))
	usedSelections := make(map[string]struct{}, len(selected))
	pathSelections := make([]string, 0, len(selected))
	for {
		if _, duplicate := visited[current.GetMessageDetailId()]; duplicate {
			return nil, nil, status.Error(codes.FailedPrecondition, "message detail graph contains a cycle")
		}
		visited[current.GetMessageDetailId()] = struct{}{}
		path = append(path, current)
		if stopId != "" && current.GetMessageDetailId() == stopId {
			if len(usedSelections) != len(selected) {
				return nil, nil, viber.InvalidArgumentf("message selections do not reach detail %q", stopId)
			}
			return path, pathSelections, nil
		}

		nextIds := current.GetNextMessageDetailIds()
		if len(nextIds) == 0 {
			if stopId != "" {
				return nil, nil, viber.InvalidArgumentf("message detail %q is not reachable with the selected path", stopId)
			}
			if len(usedSelections) != len(selected) {
				return nil, nil, viber.InvalidArgumentf("message selections do not form one complete path")
			}
			return path, pathSelections, nil
		}

		nextId := nextIds[0]
		if len(nextIds) > 1 {
			selectedCount := 0
			for _, candidateId := range nextIds {
				candidate := details[candidateId]
				if candidate.GetChoiceText() == "" {
					return nil, nil, status.Errorf(codes.FailedPrecondition, "message detail %q has a non-choice branch", current.GetMessageDetailId())
				}
				if _, ok := selected[candidateId]; ok {
					nextId = candidateId
					selectedCount++
				}
			}
			if selectedCount != 1 {
				return nil, nil, viber.InvalidArgumentf("message detail %q requires exactly one selected choice", current.GetMessageDetailId())
			}
			usedSelections[nextId] = struct{}{}
			pathSelections = append(pathSelections, nextId)
		} else if next := details[nextId]; next.GetChoiceText() != "" {
			if _, ok := selected[nextId]; !ok {
				return nil, nil, viber.InvalidArgumentf("message detail %q requires its selected choice", current.GetMessageDetailId())
			}
			usedSelections[nextId] = struct{}{}
			pathSelections = append(pathSelections, nextId)
		}
		current = details[nextId]
	}
}

func validateMessageProgressExtension(
	master *pmaster.Message,
	info *papi.TimelineInfo,
	message *ptransaction.UserMessage,
	newPath []*pmaster.MessageDetail,
) error {
	if info.GetLastMessageDetailId() == "" {
		if len(message.GetSelectMessageDetailIds()) != 0 {
			return status.Error(codes.FailedPrecondition, "message selections exist without saved progress")
		}
		return nil
	}
	savedPath, _, err := resolveMessageDetails(
		master,
		message.GetSelectMessageDetailIds(),
		info.GetLastMessageDetailId(),
	)
	if err != nil {
		return status.Errorf(codes.FailedPrecondition, "saved message progress is invalid: %v", err)
	}
	if len(newPath) < len(savedPath) {
		return status.Error(codes.FailedPrecondition, "message progress cannot be rewound")
	}
	for index, detail := range savedPath {
		if newPath[index].GetMessageDetailId() != detail.GetMessageDetailId() {
			return status.Error(codes.FailedPrecondition, "message progress cannot switch paths")
		}
	}
	return nil
}

func terminalMessageDetailId(master *pmaster.Message) string {
	result := ""
	for _, detail := range master.Details {
		if len(detail.NextMessageDetailIds) == 0 && detail.MessageDetailId > result {
			result = detail.MessageDetailId
		}
	}
	return result
}

func mergeSortedStrings(left, right []string) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	result := make([]string, 0, len(left)+len(right))
	for _, values := range [][]string{left, right} {
		for _, value := range values {
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
