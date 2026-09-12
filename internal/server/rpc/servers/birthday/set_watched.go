package birthday

import (
	"context"
	"slices"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/birthday"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *BirthdayServer) SetWatched(ctx context.Context, req *papi.BirthdaySetWatchedRequest) (*papi.BirthdaySetWatchedResponse, error) {
	if req.GetBirthdayId() == "" {
		return nil, viber.InvalidArgumentf("birthdayId is required")
	}
	var response *papi.BirthdaySetWatchedResponse
	err := withBirthday(ctx, func(ctx context.Context, _ string, event *birthday.Event) error {
		if !event.Active || !slices.Contains(event.State.BirthdayIDs, req.GetBirthdayId()) {
			return viber.FailedPreconditionf("birthday story is not available")
		}
		if !slices.Contains(event.State.WatchedIDs, req.GetBirthdayId()) {
			// The greeting order is selected at event creation and cannot be skipped.
			if event.NextAdv().GetBirthdayId() != req.GetBirthdayId() {
				return viber.FailedPreconditionf("watch the preceding birthday greeting first")
			}
			event.State.WatchedIDs = append(event.State.WatchedIDs, req.GetBirthdayId())
			if err := opuser.UpsertUserBirthdayState(ctx, event.State); err != nil {
				return err
			}
		}
		response = &papi.BirthdaySetWatchedResponse{CommonResponse: viber.NewCommonResponse(nil),
			UnwatchAdvBirthdayIds: event.UnwatchedIDs(), ReceivableRewardBirthdayIds: event.ReceivableIDs()}
		return nil
	})
	return response, err
}
