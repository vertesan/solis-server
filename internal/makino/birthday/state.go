package birthday

import (
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Period returns the most recent birthday's seven-day window. The interval is
// [birthday 00:00 JST, birthday+7 days 00:00 JST), including across New Year.
// February 29 uses February 28 in non-leap years, an uncaptured fallback.
func Period(month, day int64, now time.Time) (start, end time.Time, active bool) {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, time.Time{}, false
	}
	local := now.In(time.FixedZone("JST", 9*60*60))
	start = birthdayDate(local.Year(), month, day, local.Location())
	if local.Before(start) {
		start = birthdayDate(local.Year()-1, month, day, local.Location())
	}
	end = start.AddDate(0, 0, 7)
	return start, end, !local.Before(start) && local.Before(end)
}

func birthdayDate(year int, month, day int64, location *time.Location) time.Time {
	if month == 2 && day == 29 && time.Date(year, 3, 0, 0, 0, 0, 0, location).Day() == 28 {
		day = 28
	}
	return time.Date(year, time.Month(month), int(day), 0, 0, 0, 0, location)
}

// ReminderYear identifies the upcoming birthday during the seven JST calendar
// days before it. The reminder stops when the birthday event begins.
func ReminderYear(month, day int64, now time.Time) int {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return 0
	}
	local := now.In(time.FixedZone("JST", 9*60*60))
	next := birthdayDate(local.Year(), month, day, local.Location())
	if !local.Before(next) {
		next = birthdayDate(local.Year()+1, month, day, local.Location())
	}
	if !local.Before(next.AddDate(0, 0, -7)) && local.Before(next) {
		return next.Year()
	}
	return 0
}

type Event struct {
	State        *umodel.UserBirthdayState
	Catalog      *Catalog
	Active       bool
	ReminderYear int
}

// Reconcile is called inside the request transaction. A participant selection
// is made only once each birthday year and is never rerolled by profile edits.
func Reconcile(ctx context.Context, suid string, profile *ptransaction.UserProfile, now time.Time) (*Event, error) {
	state, err := opuser.FindUserBirthdayState(ctx, suid)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	if state == nil {
		state = &umodel.UserBirthdayState{SUID: suid}
	}
	start, _, active := Period(profile.GetBirthMonth(), profile.GetBirthDay(), now)
	reminderYear := ReminderYear(profile.GetBirthMonth(), profile.GetBirthDay(), now)
	event := &Event{State: state, Active: active, ReminderYear: reminderYear}
	if !active && reminderYear == 0 {
		return event, nil
	}
	event.Catalog, err = LoadCatalog(ctx)
	if err != nil {
		return nil, err
	}
	if active && state.EventYear != start.Year() {
		state.EventYear = start.Year()
		state.BirthdayIDs = selectParticipants(profile, event.Catalog)
		state.InFavoriteOrder = true
		state.WatchedIDs, state.ClaimedIDs = nil, nil
		if err := opuser.UpsertUserBirthdayState(ctx, state); err != nil {
			return nil, err
		}
	} else if active && !state.InFavoriteOrder {
		// Correct existing events once, without reselecting participants from
		// possibly edited favorites or resetting watched/claimed identities.
		slices.Reverse(state.BirthdayIDs)
		state.InFavoriteOrder = true
		if err := opuser.UpsertUserBirthdayState(ctx, state); err != nil {
			return nil, err
		}
	}
	return event, nil
}

func selectParticipants(profile *ptransaction.UserProfile, catalog *Catalog) []string {
	selected := make([]string, 0, 3)
	for _, characterID := range profile.GetFavoriteCharacterIds() {
		if slices.Contains(catalog.CharacterIDs, characterID) && !slices.Contains(selected, characterID) && len(selected) < 3 {
			selected = append(selected, characterID)
		}
	}
	remaining := slices.Clone(catalog.CharacterIDs)
	rand.Shuffle(len(remaining), func(i, j int) { remaining[i], remaining[j] = remaining[j], remaining[i] })
	for _, characterID := range remaining {
		if len(selected) == 3 {
			break
		}
		if !slices.Contains(selected, characterID) {
			selected = append(selected, characterID)
		}
	}
	// Preserve favorite order: #1 greets first, then #2 and #3.
	for i, characterID := range selected {
		selected[i] = "birthday-01-" + strings.TrimPrefix(characterID, "char-") + "-01"
	}
	return selected
}

func (e *Event) UnwatchedIDs() []string {
	ids := make([]string, 0)
	if e.Active {
		for _, id := range e.State.BirthdayIDs {
			if !slices.Contains(e.State.WatchedIDs, id) {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func (e *Event) ReceivableIDs() []string {
	ids := make([]string, 0)
	if e.Active {
		for _, id := range e.State.BirthdayIDs {
			if slices.Contains(e.State.WatchedIDs, id) && !slices.Contains(e.State.ClaimedIDs, id) {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func (e *Event) NextAdv() *papi.BirthdayAdvInfo {
	for _, id := range e.UnwatchedIDs() {
		if entry := e.Catalog.Entries[id]; entry != nil {
			return entry.Info.BirthdayAdvInfo
		}
	}
	return nil
}

func (e *Event) Reminder() *papi.BirthdayRemindInfo {
	if !e.Active && e.ReminderYear != 0 && e.ReminderYear != e.State.RemindedYear {
		return e.Catalog.Reminder
	}
	return nil
}

func (e *Event) HomeInfo() *papi.HomeBirthdayInfo {
	if !e.Active {
		return nil
	}
	return &papi.HomeBirthdayInfo{BannerAssetId: e.Catalog.BannerAssetID, Order: e.Catalog.Order,
		IsRemainingUnwatched: len(e.UnwatchedIDs()) != 0, IsRemainingReceivableReward: len(e.ReceivableIDs()) != 0}
}
