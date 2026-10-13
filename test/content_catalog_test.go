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

func TestHomeEventCatalogScenario(t *testing.T) {
	setupTestUser(t)
	home := profileCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	assertHomeLoveEntrances(t, home)
	if home.GetEventInfo().GetDreamInfo() == nil {
		t.Fatal("Home.Enter lost the permanent Dream entrance")
	}
	remaining := proto.Clone(home.GetEventInfo()).(*papi.HomeEventInfo)
	remaining.LoveEventInfos, remaining.DreamInfo, remaining.HomeBirthdayInfo = nil, nil, nil
	if !proto.Equal(remaining, &papi.HomeEventInfo{}) {
		t.Fatalf("Home.Enter exposed unsupported events: %v", remaining)
	}
	// Old campaign state must not make disabled missions available again.
	ctx := context.Background()
	groups, err := opmaster.FindEventMissions(ctx, []string{"event-beginner-mission"})
	if err != nil || len(groups) != 1 || len(groups[0].GetMissionIds()) == 0 {
		t.Fatalf("missing beginner event fixture: %v", err)
	}
	group := groups[0]
	id := group.GetMissionIds()[0]
	legacy := &ptransaction.UserMission{MissionId: id, CurrentThreshold: 1, Progress: 1,
		StatusType: penum.MissionStatusType_MissionStatusType_Clear, LastUpdateTime: time.Now().UnixMilli()}
	if err := opuser.UpsertUserMission(ctx, testConfig.TestUserSuid, legacy); err != nil {
		t.Fatal(err)
	}
	client := papi.NewMissionClient(testConfig.GrpcConn)
	profileReject(t, codes.FailedPrecondition, client.Event, &papi.MissionEventRequest{EventMissionId: group.GetId()})
	profileReject(t, codes.FailedPrecondition, client.Receive, &papi.MissionReceiveRequest{MissionIds: []string{id}})
	listed := profileCall(t, client.List, &papi.Empty{})
	if findUserMissionByID(listed.GetUserMissions(), id) != nil {
		t.Fatal("Mission.List exposed a disabled campaign mission")
	}
}

func TestLoginBonusCatalogScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	masters, err := opmaster.FindAllLoginBonuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	disabled := map[string]*ptransaction.UserLoginBonus{}
	for _, master := range masters {
		if !strings.Contains(master.GetName(), "スタートダッシュ") {
			continue
		}
		// Simulate an older account whose start-dash reward is still pending.
		row := &ptransaction.UserLoginBonus{LoginBonusId: master.GetId(), LoginCount: 1,
			LastLoginTime: time.Now().Add(-48 * time.Hour).UnixMilli()}
		if err := opuser.UpsertUserLoginBonus(ctx, testConfig.TestUserSuid, row); err != nil {
			t.Fatal(err)
		}
		disabled[master.GetId()] = row
	}
	if len(disabled) == 0 {
		t.Fatal("master data has no start-dash regression fixture")
	}
	client := papi.NewLoginBonusClient(testConfig.GrpcConn)
	list := profileCall(t, client.List, &papi.Empty{})
	for _, info := range list.GetInfos() {
		if disabled[info.GetUserLoginBonus().GetLoginBonusId()] != nil {
			t.Fatalf("LoginBonus.List exposed start dash: %v", info)
		}
	}
	for id, before := range disabled {
		profileReject(t, codes.PermissionDenied, client.Receive, &papi.LoginBonusReceiveRequest{LoginBonusId: id})
		after, err := opuser.FindUserLoginBonus(ctx, testConfig.TestUserSuid, id)
		if err != nil || !proto.Equal(before, after) {
			t.Fatalf("disabled start-dash state was advanced or claimed: %v %v", after, err)
		}
	}
}
