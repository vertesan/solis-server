package birthday

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/birthday"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *BirthdayServer) SetWatchedRemindInfo(ctx context.Context, _ *papi.Empty) (*papi.BirthdaySetWatchedRemindInfoResponse, error) {
	err := withBirthday(ctx, func(ctx context.Context, _ string, event *birthday.Event) error {
		if event.ReminderYear == 0 || event.State.RemindedYear == event.ReminderYear {
			return nil
		}
		event.State.RemindedYear = event.ReminderYear
		return opuser.UpsertUserBirthdayState(ctx, event.State)
	})
	if err != nil {
		return nil, err
	}
	return &papi.BirthdaySetWatchedRemindInfoResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
