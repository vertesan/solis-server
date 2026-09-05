package message

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
