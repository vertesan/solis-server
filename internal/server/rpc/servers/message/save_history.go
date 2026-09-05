package message

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
