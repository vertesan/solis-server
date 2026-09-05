package message

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
