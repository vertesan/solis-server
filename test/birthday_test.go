package test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/birthday"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func TestBirthdayEventScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewBirthdayClient(testConfig.GrpcConn)
	profileClient := papi.NewProfileClient(testConfig.GrpcConn)
	home := papi.NewHomeClient(testConfig.GrpcConn)
	before := profileCall(t, client.Top, &papi.Empty{})
	if len(before.GetBirthdayInfos()) != 0 {
		t.Fatal("an unset birthday exposed an event")
	}
	profileReject(t, codes.FailedPrecondition, client.ReceiveReward, &papi.Empty{})
	catalog, err := birthday.LoadCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Exact captions, including line breaks, from the captured Birthday.Top
	// responses plus the two captions supplied directly for kana and miho.
	for code, want := range map[string]string{
		"ai": "全力でお祝い\nしますねっ！", "aoi": "やぁやぁ\nおめでとう",
		"chs": "いっぱいお祝い\nしないとです！", "hrk": "君も立派に\nなったわね～",
		"kan": "一緒に\n写真撮ろ～！", "kkr": "こころがお祝い\nしてあげま～す",
		"kor": "お祝いのお返し\n期待してますよ", "ktn": "お、おめでとう\nございます…！",
		"mei": "おめでとーー\nーーーーー！", "mhk": "私だってお祝い\nくらいしますよ",
		"ngs": "自分のことのよ\nうに嬉しいです", "rei": "ちゃんとお祝い\nさせて下さいね",
		"rio": "一応、気持ちを\n込めたのよ", "rui": "私にとっても\n特別な日です",
		"ski": "良い一年を過ご\nせますように", "skr": "あなたに～～！\n届け～～～～！",
		"smr": "お兄ちゃん\nおめでと～！", "suz": "お、お祝いして\n差し上げますわ",
		"szk": "お誕生日…\nめでたい…！", "yu": "ふふ、何が\nほしいん？",
	} {
		entry := catalog.Entries["birthday-01-"+code+"-01"]
		if entry == nil || entry.Info.GetText() != want {
			t.Fatalf("birthday caption for %s does not match the supplied evidence: %v", code, entry)
		}
	}
	characters, err := opuser.FindAllUserCharacters(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	favorites := make([]string, 0, 3)
	for _, character := range characters {
		if slices.Contains(catalog.CharacterIDs, character.GetCharacterId()) {
			favorites = append(favorites, character.GetCharacterId())
			if len(favorites) == 3 {
				break
			}
		}
	}
	if len(favorites) != 3 {
		t.Fatal("test user requires three birthday-supported idols")
	}
	profileCall(t, profileClient.UpdateFavoriteCharacters, &papi.ProfileUpdateFavoriteCharactersRequest{FavoriteCharacterIds: favorites})
	now := time.Now().In(time.FixedZone("JST", 9*60*60))
	set := profileCall(t, profileClient.UpdateBirthday, &papi.ProfileUpdateBirthdayRequest{Month: int32(now.Month()), Day: int32(now.Day())})
	wantIDs := make([]string, 3)
	for i := range favorites {
		wantIDs[i] = "birthday-01-" + strings.TrimPrefix(favorites[i], "char-") + "-01"
	}
	if set.GetBirthdayAdvInfo().GetBirthdayId() != wantIDs[0] || set.GetBirthdayRemindInfo() != nil {
		t.Fatalf("today's birthday did not immediately offer the first favorite: %v", set)
	}
	login := profileCall(t, home.Login, &papi.HomeLoginRequest{})
	if !proto.Equal(login.GetBirthdayAdvInfo(), set.GetBirthdayAdvInfo()) {
		t.Fatal("Home.Login rerolled the birthday greeting")
	}
	entered := profileCall(t, home.Enter, &papi.HomeEnterRequest{})
	info := entered.GetEventInfo().GetHomeBirthdayInfo()
	if info == nil || !info.GetIsRemainingUnwatched() || info.GetIsRemainingReceivableReward() {
		t.Fatalf("Home.Enter birthday event flags are wrong before watching: %v", info)
	}
	profileReject(t, codes.FailedPrecondition, client.SetWatched, &papi.BirthdaySetWatchedRequest{BirthdayId: "birthday-not-owned"})
	profileReject(t, codes.FailedPrecondition, client.SetWatched, &papi.BirthdaySetWatchedRequest{BirthdayId: wantIDs[2]})
	for i, id := range wantIDs {
		watched := profileCall(t, client.SetWatched, &papi.BirthdaySetWatchedRequest{BirthdayId: id})
		if !slices.Equal(watched.GetUnwatchAdvBirthdayIds(), wantIDs[i+1:]) || !slices.Equal(watched.GetReceivableRewardBirthdayIds(), wantIDs[:i+1]) {
			t.Fatalf("birthday viewing sequence is inconsistent: %v", watched)
		}
		profileCall(t, client.OutputWatchedLog, &papi.BirthdayOutputWatchedLogRequest{BirthdayId: id, IsSkip: true})
	}
	// Replays are legal, but don't append duplicate receivable rewards.
	profileCall(t, client.SetWatched, &papi.BirthdaySetWatchedRequest{BirthdayId: wantIDs[0]})
	entered = profileCall(t, home.Enter, &papi.HomeEnterRequest{})
	info = entered.GetEventInfo().GetHomeBirthdayInfo()
	if info.GetIsRemainingUnwatched() || !info.GetIsRemainingReceivableReward() {
		t.Fatalf("Home.Enter did not offer watched birthday rewards: %v", info)
	}
	received := profileCall(t, client.ReceiveReward, &papi.Empty{})
	if len(received.GetRewards()) != 3 || len(received.GetCommonResponse().GetUpdatedData().GetDecorations()) != 3 {
		t.Fatalf("birthday did not grant exactly one decoration for each participant: %v", received)
	}
	for _, reward := range received.GetRewards() {
		if _, err := opuser.FindUserDecoration(ctx, testConfig.TestUserSuid, reward.GetResourceId()); err != nil {
			t.Fatalf("birthday decoration was not persisted: %v", err)
		}
	}
	if len(profileCall(t, client.ReceiveReward, &papi.Empty{}).GetRewards()) != 0 {
		t.Fatal("birthday rewards were granted twice")
	}
	profileCall(t, profileClient.UpdateFavoriteCharacters, &papi.ProfileUpdateFavoriteCharactersRequest{})
	top := profileCall(t, client.Top, &papi.Empty{})
	if len(top.GetBirthdayInfos()) != 3 || len(top.GetUnwatchedAdvBirthdayIds()) != 0 || len(top.GetReceivableRewardBirthdayIds()) != 0 {
		t.Fatalf("birthday progress was lost after changing favorites: %v", top)
	}
	for i, info := range top.GetBirthdayInfos() {
		if info.GetBirthdayAdvInfo().GetBirthdayId() != wantIDs[i] || info.GetCostumeId() == "" || info.GetText() == "" {
			t.Fatal("birthday participants rerolled or lost presentation data")
		}
	}
	if profileCall(t, home.Login, &papi.HomeLoginRequest{}).GetBirthdayAdvInfo() != nil {
		t.Fatal("completed birthday repeated its introductory greeting")
	}
	info = profileCall(t, home.Enter, &papi.HomeEnterRequest{}).GetEventInfo().GetHomeBirthdayInfo()
	if info == nil || info.GetIsRemainingUnwatched() || info.GetIsRemainingReceivableReward() {
		t.Fatal("completed birthday should remain visible without notification flags")
	}
}

func TestBirthdayReminderScenario(t *testing.T) {
	setupTestUser(t)
	client := papi.NewBirthdayClient(testConfig.GrpcConn)
	home := papi.NewHomeClient(testConfig.GrpcConn)
	birthDate := time.Now().In(time.FixedZone("JST", 9*60*60)).AddDate(0, 0, 7)
	set := profileCall(t, papi.NewProfileClient(testConfig.GrpcConn).UpdateBirthday,
		&papi.ProfileUpdateBirthdayRequest{Month: int32(birthDate.Month()), Day: int32(birthDate.Day())})
	if set.GetBirthdayAdvInfo() != nil || set.GetBirthdayRemindInfo().GetAssetId() != "user-birthday-advance-notice" {
		t.Fatalf("birthday in seven days should offer the favorite-character reminder: %v", set)
	}
	login := profileCall(t, home.Login, &papi.HomeLoginRequest{})
	if !proto.Equal(login.GetBirthdayRemindInfo(), set.GetBirthdayRemindInfo()) {
		t.Fatal("Home.Login omitted the outstanding birthday reminder")
	}
	if profileCall(t, home.Enter, &papi.HomeEnterRequest{}).GetEventInfo().GetHomeBirthdayInfo() != nil {
		t.Fatal("birthday event began during the reminder window")
	}
	profileCall(t, client.SetWatchedRemindInfo, &papi.Empty{})
	profileCall(t, client.SetWatchedRemindInfo, &papi.Empty{})
	if profileCall(t, home.Login, &papi.HomeLoginRequest{}).GetBirthdayRemindInfo() != nil {
		t.Fatal("dismissed birthday reminder reappeared")
	}
	state, err := opuser.FindUserBirthdayState(context.Background(), testConfig.TestUserSuid)
	if err != nil || state.RemindedYear != birthDate.Year() || state.EventYear != 0 {
		t.Fatalf("reminder acknowledgement was not stored independently of the future event: %v", err)
	}
}

func TestBirthdayLegacyOrderScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	suid := testConfig.TestUserSuid
	catalog, err := birthday.LoadCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 3)
	for i, characterID := range catalog.CharacterIDs[:3] {
		ids[i] = "birthday-01-" + strings.TrimPrefix(characterID, "char-") + "-01"
	}
	now := time.Now().In(time.FixedZone("JST", 9*60*60))
	profile, err := opuser.FindUserProfile(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	profile.BirthMonth, profile.BirthDay = int64(now.Month()), int64(now.Day())
	// Favorites may have changed since the event was selected. Its original
	// participants must be retained, not regenerated from today's profile.
	profile.FavoriteCharacterIds = nil
	if err := opuser.UpsertUserProfile(ctx, suid, profile); err != nil {
		t.Fatal(err)
	}
	legacy := &umodel.UserBirthdayState{
		SUID: suid, EventYear: now.Year(), BirthdayIDs: []string{ids[2], ids[1], ids[0]},
		WatchedIDs: []string{ids[2]}, ClaimedIDs: []string{ids[2]}, RemindedYear: now.Year(),
	}
	if err := opuser.UpsertUserBirthdayState(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	client := papi.NewBirthdayClient(testConfig.GrpcConn)
	for range 2 {
		top := profileCall(t, client.Top, &papi.Empty{})
		if len(top.GetBirthdayInfos()) != 3 || !slices.Equal(top.GetUnwatchedAdvBirthdayIds(), ids[:2]) || len(top.GetReceivableRewardBirthdayIds()) != 0 {
			t.Fatalf("legacy order correction lost viewing/reward state: %v", top)
		}
		for i, info := range top.GetBirthdayInfos() {
			if info.GetBirthdayAdvInfo().GetBirthdayId() != ids[i] {
				t.Fatal("legacy participants were not reordered exactly once")
			}
		}
	}
	stored, err := opuser.FindUserBirthdayState(ctx, suid)
	if err != nil || !stored.InFavoriteOrder || !slices.Equal(stored.WatchedIDs, legacy.WatchedIDs) || !slices.Equal(stored.ClaimedIDs, legacy.ClaimedIDs) || stored.RemindedYear != legacy.RemindedYear {
		t.Fatalf("legacy correction was not saved without resetting progress: %+v %v", stored, err)
	}
	login := profileCall(t, papi.NewHomeClient(testConfig.GrpcConn).Login, &papi.HomeLoginRequest{})
	if login.GetBirthdayAdvInfo().GetBirthdayId() != ids[0] {
		t.Fatal("Home.Login did not offer the first unwatched favorite after correction")
	}
	for _, id := range ids[:2] {
		profileCall(t, client.SetWatched, &papi.BirthdaySetWatchedRequest{BirthdayId: id})
	}
	claimed := profileCall(t, client.ReceiveReward, &papi.Empty{})
	if len(claimed.GetRewards()) != 2 {
		t.Fatal("legacy correction repeated an already claimed birthday reward")
	}
}

func TestBirthdayReminderWindowScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	jst := time.FixedZone("JST", 9*60*60)
	// January 3 also verifies that a December reminder belongs to the next
	// birthday year rather than the current calendar year.
	start := time.Date(2028, 1, 3, 0, 0, 0, 0, jst)
	profile := &ptransaction.UserProfile{BirthMonth: 1, BirthDay: 3}
	for _, offset := range []time.Duration{-7*24*time.Hour - time.Millisecond, -7 * 24 * time.Hour, -6 * 24 * time.Hour, -24 * time.Hour, -time.Millisecond, 0} {
		now := start.Add(offset).UTC()
		wantReminder := offset >= -7*24*time.Hour && offset < 0
		err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
			event, err := birthday.Reconcile(ctx, testConfig.TestUserSuid, profile, now)
			if err != nil {
				return nil, err
			}
			if (event.Reminder() != nil) != wantReminder || (wantReminder && event.ReminderYear != start.Year()) || event.Active != (offset == 0) {
				t.Fatalf("birthday reminder/event boundary is incorrect at %v: %+v", now, event)
			}
			return nil, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestBirthdayAnnualWindowScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	jst := time.FixedZone("JST", 9*60*60)
	start := time.Date(2027, 12, 29, 0, 0, 0, 0, jst)
	profile := &ptransaction.UserProfile{BirthMonth: 12, BirthDay: 29}
	var originalIDs []string
	for _, sample := range []struct {
		now    time.Time
		active bool
	}{
		{start.Add(-time.Millisecond), false},
		{start, true},
		{start.AddDate(0, 0, 7).Add(-time.Millisecond), true},
		{start.AddDate(0, 0, 7), false},
	} {
		err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
			event, err := birthday.Reconcile(ctx, testConfig.TestUserSuid, profile, sample.now)
			if err != nil {
				return nil, err
			}
			if event.Active != sample.active {
				t.Fatalf("birthday active=%v at %v, want %v", event.Active, sample.now, sample.active)
			}
			if event.Active {
				if event.State.EventYear != 2027 || len(event.State.BirthdayIDs) != 3 {
					t.Fatal("cross-year birthday lost its original event year or random participants")
				}
				if originalIDs == nil {
					originalIDs = slices.Clone(event.State.BirthdayIDs)
				} else if !slices.Equal(originalIDs, event.State.BirthdayIDs) {
					t.Fatal("random birthday participants changed between visits")
				}
				event.State.WatchedIDs = slices.Clone(originalIDs)
				event.State.ClaimedIDs = slices.Clone(originalIDs)
				return nil, opuser.UpsertUserBirthdayState(ctx, event.State)
			}
			if len(event.UnwatchedIDs()) != 0 || len(event.ReceivableIDs()) != 0 || event.HomeInfo() != nil {
				t.Fatal("inactive birthday exposed expired progress")
			}
			return nil, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		event, err := birthday.Reconcile(ctx, testConfig.TestUserSuid, profile, start.AddDate(1, 0, 0))
		if err != nil {
			return nil, err
		}
		if !event.Active || event.State.EventYear != 2028 || len(event.UnwatchedIDs()) != 3 || len(event.ReceivableIDs()) != 0 {
			t.Fatal("next year's birthday did not reset annual watch/reward state")
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, year := range []int{2027, 2028} {
		day := 28
		if year == 2028 {
			day = 29
		}
		now := time.Date(year, 2, day, 0, 0, 0, 0, jst)
		gotStart, gotEnd, active := birthday.Period(2, 29, now)
		if !active || !gotStart.Equal(now) || !gotEnd.Equal(now.AddDate(0, 0, 7)) {
			t.Fatalf("February 29 fallback is inconsistent in %d", year)
		}
	}
}
