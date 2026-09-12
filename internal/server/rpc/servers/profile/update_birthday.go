package profile

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/makino/birthday"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ProfileServer) UpdateBirthday(ctx context.Context, req *papi.ProfileUpdateBirthdayRequest) (*papi.ProfileUpdateBirthdayResponse, error) {
	month, day := req.GetMonth(), req.GetDay()
	// A leap year admits February 29 without storing or requiring a birth year.
	date := time.Date(2000, time.Month(month), int(day), 0, 0, 0, 0, time.UTC)
	if month < 1 || month > 12 || day < 1 || day > 31 || int32(date.Month()) != month || int32(date.Day()) != day {
		return nil, viber.InvalidArgumentf("birthday must be a valid month and day")
	}
	var birthdayEvent *birthday.Event
	now := time.Now()
	common, err := updateProfile(ctx, func(ctx context.Context, suid string, profile *ptransaction.UserProfile) error {
		if profile.GetBirthMonth() != 0 || profile.GetBirthDay() != 0 {
			return viber.FailedPreconditionf("birthday has already been set")
		}
		profile.BirthMonth = int64(month)
		profile.BirthDay = int64(day)
		var err error
		birthdayEvent, err = birthday.Reconcile(ctx, suid, profile, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &papi.ProfileUpdateBirthdayResponse{CommonResponse: common,
		BirthdayAdvInfo: birthdayEvent.NextAdv(), BirthdayRemindInfo: birthdayEvent.Reminder()}, nil
}
