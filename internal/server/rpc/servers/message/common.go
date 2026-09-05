package message

import (
	"context"
	"hash/fnv"
	"sort"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type messageTimelineState struct {
	infos     []*papi.TimelineInfo
	usersById map[string]*ptransaction.UserMessage
	masters   map[string]*pmaster.Message
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
