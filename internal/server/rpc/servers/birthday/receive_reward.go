package birthday

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/birthday"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *BirthdayServer) ReceiveReward(ctx context.Context, _ *papi.Empty) (*papi.BirthdayReceiveRewardResponse, error) {
	var response *papi.BirthdayReceiveRewardResponse
	err := withBirthday(ctx, func(ctx context.Context, suid string, event *birthday.Event) error {
		if !event.Active {
			return viber.FailedPreconditionf("birthday event is not active")
		}
		updated := &papi.UpdatedData{}
		response = &papi.BirthdayReceiveRewardResponse{CommonResponse: viber.NewCommonResponse(updated)}
		ids := event.ReceivableIDs()
		for _, id := range ids {
			entry := event.Catalog.Entries[id]
			if entry == nil {
				return viber.FailedPreconditionf("birthday reward is unavailable")
			}
			rewards, err := viber.GrantExchangeRewards(ctx, suid, &pmaster.RewardResource{
				ResourceType: penum.ResourceType_ResourceType_Decoration, ResourceId: entry.DecorationID, Amount: 1,
			}, updated)
			if err != nil {
				return err
			}
			response.Rewards = append(response.Rewards, rewards...)
			event.State.ClaimedIDs = append(event.State.ClaimedIDs, id)
		}
		if len(ids) != 0 {
			return opuser.UpsertUserBirthdayState(ctx, event.State)
		}
		return nil
	})
	return response, err
}
