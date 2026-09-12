package birthday

import (
	"context"

	"github.com/vertesan/solis-server/internal/makino/birthday"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *BirthdayServer) Top(ctx context.Context, _ *papi.Empty) (*papi.BirthdayTopResponse, error) {
	var response *papi.BirthdayTopResponse
	err := withBirthday(ctx, func(_ context.Context, _ string, event *birthday.Event) error {
		response = &papi.BirthdayTopResponse{CommonResponse: viber.NewCommonResponse(nil),
			UnwatchedAdvBirthdayIds: event.UnwatchedIDs(), ReceivableRewardBirthdayIds: event.ReceivableIDs()}
		if event.Active {
			for _, id := range event.State.BirthdayIDs {
				if entry := event.Catalog.Entries[id]; entry != nil {
					response.BirthdayInfos = append(response.BirthdayInfos, entry.Info)
				}
			}
		}
		return nil
	})
	return response, err
}
