package test

import (
	"context"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func TestQuestSelfRankingScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewQuestClient(testConfig.GrpcConn)
	areas, err := opmaster.FindAllAreas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var tower, contest *pmaster.Area
	for _, area := range areas {
		if area.GetType() == penum.AreaType_AreaType_Tower && len(area.GetQuestIds()) >= 2 && tower == nil {
			tower = area
		}
		if area.GetType() == penum.AreaType_AreaType_Contest && len(area.GetQuestIds()) != 0 && contest == nil {
			contest = area
		}
	}
	if tower == nil || contest == nil {
		t.Fatal("quest ranking scenario requires a tower and a contest in master data")
	}
	public, err := opuser.FindUserPublic(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	towerRequest := &papi.QuestListTowerRankingRequest{AreaId: tower.GetId()}
	initialTower := profileCall(t, client.ListTowerRanking, towerRequest)
	if initialTower.GetSelfHighestRank() != 1 || initialTower.GetSelfQuestId() != "" || initialTower.GetTotalQuestNum() != int32(len(tower.GetQuestIds())) {
		t.Fatalf("fresh tower ranking has fabricated progress or incorrect catalog size: %v", initialTower)
	}
	if len(initialTower.GetRanks()) != 1 || initialTower.GetRanks()[0].GetUserId() != public.GetPublicUserId() {
		t.Fatalf("tower ranking must expose only the requesting user: %v", initialTower.GetRanks())
	}
	contestID := contest.GetQuestIds()[0]
	contestRequest := &papi.QuestListContestRankingRequest{QuestId: contestID}
	initialContest := profileCall(t, client.ListContestRanking, contestRequest)
	if initialContest.GetSelfHighestRank() != 1 || initialContest.GetSelfScore() != 0 || len(initialContest.GetRanks()) != 1 {
		t.Fatalf("fresh contest ranking has fabricated score or other users: %v", initialContest)
	}
	const score int64 = 1234567890
	clearedAt := time.Now().UnixMilli()
	// Deliberately insert the later stage first. Ranking uses the master's
	// progression, not insertion order or another tower's progress.
	if err := opuser.UnlockUserQuestsForGod(ctx, testConfig.TestUserSuid, []*ptransaction.UserQuest{
		{QuestId: tower.GetQuestIds()[1], LastClearedTime: clearedAt},
		{QuestId: tower.GetQuestIds()[0], LastClearedTime: clearedAt},
		{QuestId: contestID, LastClearedTime: clearedAt, HighestScore: score},
	}); err != nil {
		t.Fatal(err)
	}
	advanced := profileCall(t, client.ListTowerRanking, towerRequest)
	if advanced.GetSelfQuestId() != tower.GetQuestIds()[1] || len(advanced.GetRanks()) != 1 {
		t.Fatalf("tower ranking did not select the highest cleared master stage: %v", advanced)
	}
	towerRank := advanced.GetRanks()[0]
	if towerRank.GetRank() != 1 || towerRank.GetQuestId() != advanced.GetSelfQuestId() || towerRank.GetUserId() != public.GetPublicUserId() || towerRank.GetUserName() != user.GetName() || towerRank.GetEmblemId() != user.GetEmblemId() {
		t.Fatalf("tower self identity/progress is inconsistent: %v", towerRank)
	}
	ranked := profileCall(t, client.ListContestRanking, contestRequest)
	if ranked.GetSelfScore() != score || len(ranked.GetRanks()) != 1 {
		t.Fatalf("contest ranking did not use the user's stored score: %v", ranked)
	}
	contestRank := ranked.GetRanks()[0]
	if contestRank.GetRank() != 1 || contestRank.GetScore() != score || contestRank.GetUserId() != public.GetPublicUserId() || contestRank.GetUserName() != user.GetName() || contestRank.GetEmblemId() != user.GetEmblemId() || contestRank.GetIsNPC() || contestRank.GetIsLiveResultDetailAvailable() {
		t.Fatalf("contest ranking exposed fabricated identity or a nonexistent replay: %v", contestRank)
	}
	if len(towerRank.GetCards()) != 0 || len(contestRank.GetCards()) != 0 || towerRank.GetDeckName() != "" || contestRank.GetDeckName() != "" {
		t.Fatal("rankings fabricated a historical unit without a stored live result")
	}
	latest := profileCall(t, client.ListLatestClear, &papi.QuestListLatestClearRequest{QuestId: tower.GetQuestIds()[1]})
	if len(latest.GetClears()) != 0 {
		t.Fatalf("latest clear list exposed other managers: %v", latest.GetClears())
	}
	for _, userID := range []string{public.GetPublicUserId(), "NORESULT"} {
		profileReject(t, codes.NotFound, client.GetLatestClearLiveResult, &papi.QuestGetLatestClearLiveResultRequest{QuestId: tower.GetQuestIds()[1], UserId: userID, IsDetail: true})
		profileReject(t, codes.NotFound, client.GetRankingLiveResult, &papi.QuestGetRankingLiveResultRequest{QuestId: contestID, UserId: userID, IsDetail: true})
	}
	before, err := opuser.FindAllUserQuests(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	profileCall(t, client.OutputLiveResultDetailLog, &papi.OutputLiveResultDetailLogRequest{LiveResultDetailLogId: "untrusted-client-log", Cleared: true, Score: score * 100})
	profileCall(t, client.OutputLiveResultDetailLog, &papi.OutputLiveResultDetailLogRequest{})
	after, err := opuser.FindAllUserQuests(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatal("telemetry changed saved quest progress")
	}
	beforeByID := make(map[string]*ptransaction.UserQuest, len(before))
	for _, row := range before {
		beforeByID[row.GetQuestId()] = row
	}
	for _, row := range after {
		if !proto.Equal(beforeByID[row.GetQuestId()], row) {
			t.Fatalf("telemetry changed saved quest progress for %s", row.GetQuestId())
		}
	}
	profileReject(t, codes.InvalidArgument, client.ListTowerRanking, &papi.QuestListTowerRankingRequest{})
	profileReject(t, codes.InvalidArgument, client.ListTowerRanking, &papi.QuestListTowerRankingRequest{AreaId: contest.GetId()})
	profileReject(t, codes.NotFound, client.ListTowerRanking, &papi.QuestListTowerRankingRequest{AreaId: "missing-quest-area"})
	profileReject(t, codes.InvalidArgument, client.ListContestRanking, &papi.QuestListContestRankingRequest{})
	profileReject(t, codes.InvalidArgument, client.ListContestRanking, &papi.QuestListContestRankingRequest{QuestId: tower.GetQuestIds()[0]})
	profileReject(t, codes.NotFound, client.ListLatestClear, &papi.QuestListLatestClearRequest{QuestId: "missing-quest"})
}
