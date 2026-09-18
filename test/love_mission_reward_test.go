package test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func TestLoveTrueEndingMissionRewardScenario(t *testing.T) {
	setupTestUser(t)
	client, catalog := registerTestLove(t)
	ctx := context.Background()
	const missionID = "event-love-23-0514-limited-mission_love-end_emb-sp"
	const emblemID = "emb-love-23-0514-true-sp"
	masters, err := opmaster.FindMissionsByIds(ctx, []string{missionID})
	if err != nil || len(masters) != 1 {
		t.Fatalf("missing TRUE-ending mission: %v %v", masters, err)
	}
	master := masters[0]
	// The official daily-page group intentionally includes this one-time
	// mission. Its own Normal category, not the group name, controls resets.
	if master.GetCategoryType() != penum.MissionCategoryType_MissionCategoryType_Normal ||
		!slices.Contains(catalog.Config.LimitedEventMissionIDs, master.GetEventMissionId()) ||
		len(master.GetProgresses()) != 1 ||
		master.GetProgresses()[0].GetRewardSetId() != "rd_auto;emblem;"+emblemID+";1" {
		t.Fatalf("unexpected TRUE-ending mission classification or reward: %v", master)
	}
	groups, err := opmaster.FindEventMissions(ctx, []string{master.GetEventMissionId()})
	if err != nil || len(groups) != 1 || !slices.Contains(groups[0].GetMissionIds(), missionID) {
		t.Fatalf("mission is absent from its master-defined event group: %v %v", groups, err)
	}
	assertListed := func(wantStatus penum.MissionStatusType) {
		t.Helper()
		top := loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testLoveID})
		for _, group := range top.GetLimitedEventMissionInfos() {
			if group.GetId() == master.GetEventMissionId() {
				row := findUserMissionByID(group.GetUserMissions(), missionID)
				if row == nil || row.GetStatusType() != wantStatus {
					t.Fatalf("wrong TRUE-ending mission state in Love.Top: %v", row)
				}
				return
			}
		}
		t.Fatal("Love.Top omitted the mission's master-defined group")
	}
	assertListed(penum.MissionStatusType_MissionStatusType_Progress)
	missionClient := papi.NewMissionClient(testConfig.GrpcConn)
	request := &papi.MissionReceiveRequest{MissionIds: []string{missionID}}
	loveReject(t, codes.FailedPrecondition, missionClient.Receive, request)
	if _, err := opuser.FindUserEmblem(ctx, testConfig.TestUserSuid, emblemID); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("uncompleted mission already granted its emblem: %v", err)
	}

	raiseTestLoveLevel(t, catalog)
	setLoveItem(t, catalog.Config.StoryReadItemID, 1_000_000)
	lovePlayEpisode(t, client, "001", 2, 1)
	lovePlayEpisode(t, client, "002", 1, 2)
	lovePlayEpisode(t, client, "003", 1, 1)
	lovePlayEpisode(t, client, "004", 1, 2)
	lovePlayEpisode(t, client, "005", 1, 2)
	lovePlayEpisode(t, client, "006", 2, 2, 1)
	lovePlayEpisode(t, client, "007", 2, 1)
	lovePlayEpisode(t, client, "008-happy", 2)
	assertListed(penum.MissionStatusType_MissionStatusType_NotReceived)

	// A later invalid claim must roll back the earlier emblem grant as well
	// as the mission receipt in the same transaction.
	loveReject(t, codes.FailedPrecondition, missionClient.Receive, &papi.MissionReceiveRequest{
		MissionIds: []string{missionID, "not-an-active-love-mission"},
	})
	if _, err := opuser.FindUserEmblem(ctx, testConfig.TestUserSuid, emblemID); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("rejected batch partially granted the emblem: %v", err)
	}
	assertListed(penum.MissionStatusType_MissionStatusType_NotReceived)
	claim := loveCall(t, missionClient.Receive, request)
	if len(claim.GetResults()) != 1 {
		t.Fatalf("unexpected reward results: %v", claim.GetResults())
	}
	result := claim.GetResults()[0]
	if result.GetResourceType() != penum.ResourceType_ResourceType_Emblem || result.GetResourceId() != emblemID ||
		result.GetBeforeAmount() != 0 || result.GetAfterAmount() != 1 || result.GetAmountI64() != 1 {
		t.Fatalf("wrong emblem reward result: %v", result)
	}
	emblem, err := opuser.FindUserEmblem(ctx, testConfig.TestUserSuid, emblemID)
	if err != nil || !slices.ContainsFunc(claim.GetCommonResponse().GetUpdatedData().GetEmblems(), func(row *ptransaction.UserEmblem) bool {
		return proto.Equal(row, emblem)
	}) {
		t.Fatalf("emblem grant was not persisted and returned: %v %v", emblem, err)
	}
	received, err := opuser.FindUserMission(ctx, testConfig.TestUserSuid, missionID)
	if err != nil || received.GetStatusType() != penum.MissionStatusType_MissionStatusType_Received ||
		!proto.Equal(received, findUserMissionByID(claim.GetCommonResponse().GetUpdatedData().GetMissions(), missionID)) {
		t.Fatalf("mission receipt was not persisted and returned: %v %v", received, err)
	}
	loveReject(t, codes.FailedPrecondition, missionClient.Receive, request)

	// Simulate a prior-day receipt: opening Home/Love must not make the
	// one-time reward claimable again, despite its daily-page placement.
	received.LastUpdateTime = time.Now().Add(-48 * time.Hour).UnixMilli()
	if err := opuser.UpsertUserMission(ctx, testConfig.TestUserSuid, received); err != nil {
		t.Fatal(err)
	}
	loveCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	assertListed(penum.MissionStatusType_MissionStatusType_Received)
	loveReject(t, codes.FailedPrecondition, missionClient.Receive, request)
	user := loveCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	if !proto.Equal(received, findUserMissionByID(user.GetMissions(), missionID)) ||
		!slices.ContainsFunc(user.GetEmblems(), func(row *ptransaction.UserEmblem) bool { return proto.Equal(row, emblem) }) {
		t.Fatal("User.Get lost the permanent mission receipt or emblem")
	}
	emblems, err := opuser.FindAllUserEmblems(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range emblems {
		if row.GetEmblemId() == emblemID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("emblem granted %d times, want exactly one", count)
	}
}
