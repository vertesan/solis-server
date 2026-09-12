package birthday

import (
	"context"
	"slices"

	"github.com/vertesan/solis-server/internal/makino/birthday"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
)

func (s *BirthdayServer) OutputWatchedLog(ctx context.Context, req *papi.BirthdayOutputWatchedLogRequest) (*papi.BirthdayOutputWatchedLogResponse, error) {
	if req.GetBirthdayId() == "" {
		return nil, viber.InvalidArgumentf("birthdayId is required")
	}
	err := withBirthday(ctx, func(ctx context.Context, _ string, event *birthday.Event) error {
		if !event.Active || !slices.Contains(event.State.BirthdayIDs, req.GetBirthdayId()) {
			return viber.FailedPreconditionf("birthday story is not available")
		}
		// Telemetry is separate from SetWatched; replaying or skipping never
		// grants another reward or changes the saved viewing sequence.
		mcontext.GetLogger(ctx).Infow("birthday story viewed", "birthday_id", req.GetBirthdayId(), "is_skip", req.GetIsSkip())
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &papi.BirthdayOutputWatchedLogResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
