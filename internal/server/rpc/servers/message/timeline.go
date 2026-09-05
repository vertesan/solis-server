package message

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

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

const messageTimelinePageSize = 30
