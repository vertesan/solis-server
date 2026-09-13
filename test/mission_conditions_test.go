package test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func TestMissionNestedConditionScenario(t *testing.T) {
	setupTestUser(t)
	love, catalog := registerTestLove(t)
	client := papi.NewMissionClient(testConfig.GrpcConn)
	ctx := context.Background()
	const missionID = "event-love-23-0514-normal-mission_story_count-02"
	masters, err := opmaster.FindMissionsByIds(ctx, []string{missionID})
	if err != nil || len(masters) != 1 {
		t.Fatalf("load second-episode mission: %v %v", masters, err)
	}
	master := masters[0]
	view, err := opmaster.FindCondition(ctx, master.GetViewConditionId())
	if err != nil || len(view.GetSettings()) != 1 {
		t.Fatalf("load mission view condition: %v %v", view, err)
	}
	dependencyID := view.GetSettings()[0].GetSatisfyCondition().GetConditionId()
	dependency, err := opmaster.FindCondition(ctx, dependencyID)
	if err != nil || dependencyID == "" || len(dependency.GetSettings()) != 1 || dependency.GetSettings()[0].GetReadStory().GetStoryId() == "" {
		t.Fatalf("mission no longer uses a nested read-story condition: %v %v", dependency, err)
	}
	// Batched master reads must preserve payloads and missing-ID behavior.
	conditions, err := opmaster.FindConditions(ctx, []string{view.GetId(), dependencyID, dependencyID, "missing-mission-condition"})
	if err != nil || len(conditions) != 2 {
		t.Fatalf("batch condition lookup returned unexpected rows: %v %v", conditions, err)
	}
	for _, condition := range conditions {
		if !proto.Equal(condition, view) && !proto.Equal(condition, dependency) {
			t.Fatalf("batch condition differs from single lookup: %v", condition)
		}
	}
	empty, err := opmaster.FindConditions(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty condition lookup returned a catalog: %v %v", empty, err)
	}

	eventRequest := &papi.MissionEventRequest{EventMissionId: master.GetEventMissionId()}
	before := profileCall(t, client.Event, eventRequest)
	if findUserMissionByID(before.GetInfo().GetUserMissions(), missionID) != nil {
		t.Fatal("nested story gate exposed the second-episode mission before the first story was read")
	}
	profileReject(t, codes.FailedPrecondition, client.Receive, &papi.MissionReceiveRequest{MissionIds: []string{missionID}})

	lovePlayEpisode(t, love, "001", 2, 1)
	visible := profileCall(t, client.Event, eventRequest)
	row := findUserMissionByID(visible.GetInfo().GetUserMissions(), missionID)
	if row == nil || row.GetProgress() != 0 || row.GetStatusType() != penum.MissionStatusType_MissionStatusType_Progress {
		t.Fatalf("nested condition was not refreshed after reading its dependency: %v", row)
	}
	raiseTestLoveLevel(t, catalog)
	episode := catalog.Episode(loveEpisode("002"))
	if episode == nil {
		t.Fatal("current master has no second Love episode")
	}
	setLoveItem(t, catalog.Config.StoryReadItemID, int64(episode.GetConsumptionItemAmount()))
	lovePlayEpisode(t, love, "002", 1, 2)
	claimed := profileCall(t, client.Receive, &papi.MissionReceiveRequest{MissionIds: []string{missionID}})
	assertMissionClaimResources(t, claimed, master.GetProgresses()[0].GetRewardSetId())
	profileReject(t, codes.FailedPrecondition, client.Receive, &papi.MissionReceiveRequest{MissionIds: []string{missionID}})
}

func TestMissionFilteredUserStateScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewMissionClient(testConfig.GrpcConn)
	listed := profileCall(t, client.List, &papi.Empty{})
	_, received := selectListedMission(t, listed.GetUserMissions(), penum.MissionType_MissionType_ProfileSetting, penum.MissionCategoryType_MissionCategoryType_Normal)
	received.Progress = received.GetCurrentThreshold()
	received.StatusType = penum.MissionStatusType_MissionStatusType_Received
	received.LastUpdateTime = time.Now().UnixMilli()
	masters, err := opmaster.FindAllMissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var inactive *ptransaction.UserMission
	for _, master := range masters {
		if strings.HasPrefix(master.GetEventMissionId(), "event-love-23-1114-") && len(master.GetProgresses()) > 0 {
			inactive = &ptransaction.UserMission{
				MissionId: master.GetId(), CurrentThreshold: master.GetProgresses()[0].GetThreshold(),
				StatusType: penum.MissionStatusType_MissionStatusType_Progress, LastUpdateTime: time.Now().UnixMilli(),
			}
			break
		}
	}
	if inactive == nil {
		t.Fatal("current master has no unsupported Love mission fixture")
	}
	if err := opuser.UpsertManyUserMissions(ctx, testConfig.TestUserSuid, []*ptransaction.UserMission{received, inactive}); err != nil {
		t.Fatal(err)
	}
	all, err := opuser.FindAllUserMissions(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := opuser.FindUserMissionsByIds(ctx, testConfig.TestUserSuid, []string{received.GetMissionId(), inactive.GetMissionId(), inactive.GetMissionId(), "missing-user-mission"})
	if err != nil || len(selected) != 2 {
		t.Fatalf("ID-filtered mission lookup returned unexpected rows: %v %v", selected, err)
	}
	for _, row := range selected {
		if !proto.Equal(row, findUserMissionByID(all, row.GetMissionId())) {
			t.Fatalf("filtered mission lookup changed a row: %v", row)
		}
	}
	for _, statuses := range [][]penum.MissionStatusType{
		{penum.MissionStatusType_MissionStatusType_Progress, penum.MissionStatusType_MissionStatusType_Clear, penum.MissionStatusType_MissionStatusType_NotReceived},
		{penum.MissionStatusType_MissionStatusType_NotReceived},
		{penum.MissionStatusType_MissionStatusType_Received},
	} {
		filtered, err := opuser.FindUserMissionsByStatuses(ctx, testConfig.TestUserSuid, statuses)
		if err != nil {
			t.Fatal(err)
		}
		expected := 0
		for _, row := range all {
			for _, status := range statuses {
				if row.GetStatusType() == status {
					expected++
					if !proto.Equal(row, findUserMissionByID(filtered, row.GetMissionId())) {
						t.Fatalf("status-filtered lookup lost or changed mission %q", row.GetMissionId())
					}
				}
			}
		}
		if len(filtered) != expected {
			t.Fatalf("status-filtered lookup returned %d rows, expected %d", len(filtered), expected)
		}
	}
	emptyIDs, err := opuser.FindUserMissionsByIds(ctx, testConfig.TestUserSuid, nil)
	if err != nil || len(emptyIDs) != 0 {
		t.Fatalf("empty mission IDs returned a catalog: %v %v", emptyIDs, err)
	}
	emptyStatuses, err := opuser.FindUserMissionsByStatuses(ctx, testConfig.TestUserSuid, nil)
	if err != nil || len(emptyStatuses) != 0 {
		t.Fatalf("empty mission statuses returned a catalog: %v %v", emptyStatuses, err)
	}

	profileCall(t, papi.NewProfileClient(testConfig.GrpcConn).UpdateMessage, &papi.ProfileUpdateMessageRequest{Message: "Keep received mission unchanged"})
	profileCall(t, client.List, &papi.Empty{})
	profileCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	for _, expected := range []*ptransaction.UserMission{received, inactive} {
		stored, err := opuser.FindUserMission(ctx, testConfig.TestUserSuid, expected.GetMissionId())
		if err != nil || !proto.Equal(stored, expected) {
			t.Fatalf("reconciliation changed a received or inactive mission: %v, want %v (%v)", stored, expected, err)
		}
	}
}
