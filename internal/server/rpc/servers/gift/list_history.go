package gift

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *GiftServer) ListHistory(ctx context.Context, _ *papi.Empty) (*papi.GiftHistoryListResponse, error) {
	suid := viber.GetSuid(ctx).String()
	history, err := opuser.FindAllUserGiftHistories(ctx, suid)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(history, func(i, j int) bool {
		if history[i].GetReceivedTime() == history[j].GetReceivedTime() {
			return history[i].GetGiftId() < history[j].GetGiftId()
		}
		return history[i].GetReceivedTime() > history[j].GetReceivedTime()
	})
	if len(history) > giftPageSize {
		history = history[:giftPageSize]
	}
	return &papi.GiftHistoryListResponse{Gifts: history, CommonResponse: viber.NewCommonResponse(nil)}, nil
}
