package test

import (
	"testing"

	"github.com/vertesan/solis-server/internal/proto/papi"
)

func TestLoveNegativeRouteResetScenario(t *testing.T) {
	setupTestUser(t)
	client, catalog := registerTestLove(t)
	// The first unfavorable choice may put the signed route score below zero.
	// Expose a valid home-action tier, but retain the score for reset accounting.
	for range 2 {
		loveCall(t, client.Start, &papi.LoveStartRequest{StoryEpisodeId: loveEpisode("001")})
		finish := loveCall(t, client.Finish, loveFinishRequest("001", 1, 1))
		for _, love := range finish.GetCommonResponse().GetUpdatedData().GetLoves() {
			if love.GetLoveId() == testLoveID && love.GetHiddenFavorability() < 0 {
				t.Fatal("negative route score left no matching master home-action tier")
			}
		}
		top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testLoveID})
		if top.GetHiddenFavorability() != 0 {
			t.Fatalf("initial unfavorable route should expose the lowest home tier: %d", top.GetHiddenFavorability())
		}
		user := loveCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
		for _, love := range user.GetLoves() {
			if love.GetLoveId() == testLoveID && love.GetHiddenFavorability() != 0 {
				t.Fatal("User.Get does not agree with the Love home tier")
			}
		}
		loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testLoveID})
		if storedLove(t).GetHiddenFavorability() != 0 {
			t.Fatal("negative-choice/reset cycle generated favorability")
		}
	}
	setLoveItem(t, catalog.Config.PresentItemID, 1)
	loveCall(t, client.Present, &papi.LovePresentRequest{LoveId: testLoveID})
	loveCall(t, client.Reset, &papi.LoveResetRequest{LoveId: testLoveID})
	if storedLove(t).GetHiddenFavorability() != catalog.Config.PresentFavorability {
		t.Fatal("reset removed the permanent present bonus")
	}
}
