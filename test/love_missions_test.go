package test

import (
	"context"
	"slices"
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

func TestLoveMissionDailyScenario(t *testing.T) {
	setupTestUser(t)
	client, catalog := registerTestLove(t)
	ctx := context.Background()
	masters, err := opmaster.FindAllMissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var dailyLoginID, dailyWorkID string
	for _, master := range masters {
		if !slices.Contains(catalog.Config.LimitedEventMissionIDs, master.GetEventMissionId()) ||
			master.GetCategoryType() != penum.MissionCategoryType_MissionCategoryType_Daily {
			continue
		}
		if master.GetMissionType() == penum.MissionType_MissionType_LoginCount {
			dailyLoginID = master.GetId()
		} else if dailyWorkID == "" {
			dailyWorkID = master.GetId()
		}
	}
	if dailyLoginID == "" || dailyWorkID == "" {
		t.Fatal("master event lacks daily login/activity missions")
	}
	// Simulate yesterday's claimed missions, using only the configured user's rows.
	for _, id := range []string{dailyLoginID, dailyWorkID} {
		if err := opuser.UpsertUserMission(ctx, testConfig.TestUserSuid, &ptransaction.UserMission{
			MissionId: id, CurrentThreshold: 1, Progress: 99,
			StatusType:     penum.MissionStatusType_MissionStatusType_Received,
			LastUpdateTime: time.Now().Add(-48 * time.Hour).UnixMilli(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Home must refresh daily event missions before the player starts other work.
	loveCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	first, err := opuser.FindAllUserMissions(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	find := func(values []*ptransaction.UserMission, id string) *ptransaction.UserMission {
		for _, mission := range values {
			if mission.GetMissionId() == id {
				return mission
			}
		}
		return nil
	}
	if login, work := find(first, dailyLoginID), find(first, dailyWorkID); login.GetProgress() != 1 ||
		work.GetProgress() != 0 || work.GetStatusType() != penum.MissionStatusType_MissionStatusType_Progress {
		t.Fatalf("Home did not reset daily missions and award one login: login=%v work=%v", login, work)
	}
	loveCall(t, client.Top, &papi.LoveTopRequest{LoveId: testLoveID})
	second, err := opuser.FindAllUserMissions(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(find(first, dailyLoginID), find(second, dailyLoginID)) ||
		!proto.Equal(find(first, dailyWorkID), find(second, dailyWorkID)) {
		t.Fatal("re-entering Love repeated the daily reset/login credit")
	}
	// Reading the underlying story cannot bypass choices/point consumption.
	loveReject(t, codes.FailedPrecondition, papi.NewStoryClient(testConfig.GrpcConn).Read,
		&papi.StoryReadRequest{StoryId: catalog.Episode(loveEpisode("001")).GetStoryId()})
	lovePlayEpisode(t, client, "001", 2, 1)
	missions, err := opuser.FindAllUserMissions(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, master := range masters {
		if master.GetEventMissionId() != catalog.Config.EventMissionIDs[0] ||
			master.GetMissionType() != penum.MissionType_MissionType_ConditionClear {
			continue
		}
		for _, target := range master.GetTargetIds() {
			condition, err := opmaster.FindCondition(ctx, target)
			if err != nil {
				t.Fatal(err)
			}
			for _, setting := range condition.GetSettings() {
				if setting.GetReadStory().GetStoryId() == catalog.Episode(loveEpisode("001")).GetStoryId() {
					if find(missions, master.GetId()).GetProgress() != 1 {
						t.Fatal("finishing episode one did not advance its master-defined story mission")
					}
				}
			}
		}
	}
}
