package test

import (
	"context"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

func TestNotificationDefaultsScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	suid := testConfig.TestUserSuid
	user, err := opuser.FindUser(dbCtx, suid)
	if err != nil {
		t.Fatal(err)
	}
	if user.GetInviteHostUserId() != "00000000" {
		t.Fatalf("new user's inviter ID = %q, want 00000000", user.GetInviteHostUserId())
	}
	assertStoredShowcaseLikeCount := func(wantDaily int64) {
		t.Helper()
		like, err := opuser.FindUserShowcaseLike(dbCtx, suid)
		if err != nil {
			t.Fatal(err)
		}
		if like.GetRewardLikeCount() != 0 || like.GetDailyLikeCount() != wantDaily {
			t.Fatalf("stored showcase likes = %v, want rewardLikeCount=0 and dailyLikeCount=%d", like, wantDaily)
		}
	}
	assertStoredShowcaseLikeCount(0)

	userClient := papi.NewUserClient(testConfig.GrpcConn)
	fresh := profileCall(t, userClient.Get, &papi.Empty{})
	if fresh.GetUser().GetInviteHostUserId() != "00000000" || fresh.GetShowcaseLike() == nil || fresh.GetShowcaseLike().GetRewardLikeCount() != 0 {
		t.Fatalf("User.Get returned notification-triggering defaults: user=%v showcaseLike=%v", fresh.GetUser(), fresh.GetShowcaseLike())
	}

	// Existing accounts may still hold the old initial balance of 30 hearts.
	// Reconciliation must persist the change without changing unrelated counters.
	if err := opuser.UpsertUserShowcaseLike(dbCtx, suid, &ptransaction.UserShowcaseLike{
		RewardLikeCount: 30,
		DailyLikeCount:  7,
	}); err != nil {
		t.Fatal(err)
	}
	existing := profileCall(t, userClient.Get, &papi.Empty{})
	if existing.GetShowcaseLike() == nil || existing.GetShowcaseLike().GetRewardLikeCount() != 0 || existing.GetShowcaseLike().GetDailyLikeCount() != 7 {
		t.Fatalf("User.Get did not clear the legacy heart balance: %v", existing.GetShowcaseLike())
	}
	assertStoredShowcaseLikeCount(7)

	// Exercise the daily-login path without advancing the authentication clock.
	user, err = opuser.FindUser(dbCtx, suid)
	if err != nil {
		t.Fatal(err)
	}
	user.LastLoginTime = time.Now().Add(-24 * time.Hour).UnixMilli()
	if err := opuser.UpsertUser(dbCtx, suid, user); err != nil {
		t.Fatal(err)
	}
	homeClient := papi.NewHomeClient(testConfig.GrpcConn)
	login := profileCall(t, homeClient.Login, &papi.HomeLoginRequest{})
	if login.GetCommonResponse().GetUpdatedData().GetShowcaseLike().GetRewardLikeCount() != 0 {
		t.Fatal("daily Home.Login restored showcase hearts")
	}
	assertStoredShowcaseLikeCount(7)
	profileCall(t, homeClient.Enter, &papi.HomeEnterRequest{})
	assertStoredShowcaseLikeCount(7)
	afterLogin := profileCall(t, userClient.Get, &papi.Empty{})
	if afterLogin.GetShowcaseLike() == nil || afterLogin.GetShowcaseLike().GetRewardLikeCount() != 0 {
		t.Fatalf("showcase hearts recovered after daily login: %v", afterLogin.GetShowcaseLike())
	}
}
