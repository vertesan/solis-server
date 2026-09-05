package message

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

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
