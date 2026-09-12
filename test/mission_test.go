package test

import (
	"context"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestMissionProfileClaimScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewMissionClient(testConfig.GrpcConn)
	listed := profileCall(t, client.List, &papi.Empty{})
	master, row := selectListedMission(t, listed.GetUserMissions(), penum.MissionType_MissionType_ProfileSetting, penum.MissionCategoryType_MissionCategoryType_Normal)
	if row.GetProgress() != 0 {
		t.Fatalf("fresh profile mission already progressed: %v", row)
	}
	profileCall(t, papi.NewProfileClient(testConfig.GrpcConn).UpdateMessage, &papi.ProfileUpdateMessageRequest{Message: "Mission scenario"})
	ready, err := findUserMission(ctx, master.GetId())
	if err != nil || ready.GetProgress() < ready.GetCurrentThreshold() {
		t.Fatalf("profile change did not complete active mission: %v %v", ready, err)
	}
	before, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.InvalidArgument, client.Receive, &papi.MissionReceiveRequest{MissionIds: []string{master.GetId(), master.GetId()}})
	profileReject(t, codes.FailedPrecondition, client.Receive, &papi.MissionReceiveRequest{MissionIds: []string{master.GetId(), "not-an-active-mission"}})
	afterFailure, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	stillReady, missionErr := findUserMission(ctx, master.GetId())
	if err != nil || missionErr != nil || !proto.Equal(before, afterFailure) || !proto.Equal(ready, stillReady) {
		t.Fatal("rejected batch partially granted a reward or consumed a claim")
	}
	// Concurrent submissions cannot pay the same final tier twice, even when
	// neither request carries x-app-request-id.
	type result struct {
		response *papi.MissionReceiveResponse
		err      error
	}
	results := make(chan result, 2)
	for range 2 {
		rpcCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cancel)
		go func() {
			response, err := client.Receive(rpcCtx, &papi.MissionReceiveRequest{MissionIds: []string{master.GetId()}})
			results <- result{response, err}
		}()
	}
	var claimed *papi.MissionReceiveResponse
	success := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			success++
			claimed = result.response
		} else if status.Code(result.err) != codes.FailedPrecondition {
			t.Fatalf("concurrent claim failed unexpectedly: %v", result.err)
		}
	}
	if success != 1 {
		t.Fatalf("mission paid %d concurrent claims, want 1", success)
	}
	assertMissionClaimResources(t, claimed, master.GetProgresses()[0].GetRewardSetId())
	stored, err := findUserMission(ctx, master.GetId())
	if err != nil || stored.GetStatusType() != penum.MissionStatusType_MissionStatusType_Received || stored.GetLastUpdateTime() != ready.GetLastUpdateTime() {
		t.Fatalf("receipt changed gameplay timestamp or did not persist completion: %v %v", stored, err)
	}
	profileReject(t, codes.FailedPrecondition, client.Receive, &papi.MissionReceiveRequest{MissionIds: []string{master.GetId()}})
	transition := profileCall(t, client.URLTransition, &papi.MissionURLTransitionRequest{Url: "https://example.invalid/ignored"})
	if !proto.Equal(transition.GetCommonResponse(), &papi.CommonResponse{UpdatedData: &papi.UpdatedData{}, DeletedData: &papi.DeletedData{}}) {
		t.Fatal("URLTransition was not a non-mutating empty common response")
	}
	get := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	if !proto.Equal(findUserMissionByID(get.GetMissions(), master.GetId()), stored) {
		t.Fatal("User.Get did not retain the received mission")
	}
}

func TestMissionTierClaimScenario(t *testing.T) {
	setupTestUser(t)
	client := papi.NewMissionClient(testConfig.GrpcConn)
	listed := profileCall(t, client.List, &papi.Empty{})
	master, row := selectListedMission(t, listed.GetUserMissions(), penum.MissionType_MissionType_CardLevelAchievement, penum.MissionCategoryType_MissionCategoryType_Normal)
	tiers := slices.Clone(master.GetProgresses())
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].GetThreshold() < tiers[j].GetThreshold() })
	if len(tiers) < 3 {
		t.Fatal("card-level mission needs three current master tiers for this scenario")
	}
	row.CurrentThreshold = tiers[0].GetThreshold()
	row.Progress = tiers[1].GetThreshold()
	row.StatusType = penum.MissionStatusType_MissionStatusType_NotReceived
	row.LastUpdateTime = time.Now().UnixMilli()
	if err := opuser.UpsertUserMission(context.Background(), testConfig.TestUserSuid, row); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		response := profileCall(t, client.Receive, &papi.MissionReceiveRequest{MissionIds: []string{master.GetId()}})
		assertMissionClaimResources(t, response, tiers[i].GetRewardSetId())
		stored, err := findUserMission(context.Background(), master.GetId())
		wantStatus := penum.MissionStatusType_MissionStatusType_Clear
		if i == 1 {
			wantStatus = penum.MissionStatusType_MissionStatusType_Progress
		}
		if err != nil || stored.GetCurrentThreshold() != tiers[i+1].GetThreshold() || stored.GetProgress() != row.GetProgress() || stored.GetStatusType() != wantStatus || stored.GetLastUpdateTime() != row.GetLastUpdateTime() {
			t.Fatalf("claim did not advance exactly one master tier: %v %v", stored, err)
		}
	}
	profileReject(t, codes.FailedPrecondition, client.Receive, &papi.MissionReceiveRequest{MissionIds: []string{master.GetId()}})
}

func TestMissionManagerExpRewardScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	ctx := context.Background()
	client := papi.NewMissionClient(testConfig.GrpcConn)
	// Place this configured user's manager one EXP below the existing
	// dedicated-photo gate, without changing any master definitions.
	gate, err := opmaster.FindManagerLevel(ctx, 80)
	if err != nil {
		t.Fatal(err)
	}
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	user.ManagerLevel, user.ManagerExp = 79, gate.GetRequiredExp()-1
	if err := opuser.UpsertUser(ctx, testConfig.TestUserSuid, user); err != nil {
		t.Fatal(err)
	}
	photo := &ptransaction.UserPhoto{PhotoId: "mission-manager-exp-photo", Level: 1, Abilities: []*ptransaction.UserPhotoAbility{{MissionId: "ms-photo-manager_level_achievement-80"}}}
	if err := opuser.UpsertUserPhoto(ctx, testConfig.TestUserSuid, photo); err != nil {
		t.Fatal(err)
	}
	listed := profileCall(t, client.List, &papi.Empty{})
	master, row := selectListedMission(t, listed.GetUserMissions(), penum.MissionType_MissionType_ContestTotalScoreAchievement, penum.MissionCategoryType_MissionCategoryType_Normal)
	var rewardID string
	var exp int64
	for _, tier := range master.GetProgresses() {
		resources, err := resolveCatalogRewards(ctx, tier.GetRewardSetId())
		if err != nil {
			t.Fatal(err)
		}
		for _, resource := range resources {
			if resource.GetResourceType() == penum.ResourceType_ResourceType_ManagerExp {
				exp += int64(resource.GetAmount())
			}
		}
		if exp > 0 {
			rewardID = tier.GetRewardSetId()
			row.CurrentThreshold = tier.GetThreshold()
			break
		}
	}
	if rewardID == "" {
		t.Fatal("current master Contest achievement has no ManagerExp reward")
	}
	row.Progress, row.StatusType, row.LastUpdateTime = row.GetCurrentThreshold(), penum.MissionStatusType_MissionStatusType_Clear, time.Now().UnixMilli()
	if err := opuser.UpsertUserMission(ctx, testConfig.TestUserSuid, row); err != nil {
		t.Fatal(err)
	}
	claim := profileCall(t, client.Receive, &papi.MissionReceiveRequest{MissionIds: []string{master.GetId()}})
	assertEligibleSalaryMutationSnapshot(t)
	assertMissionClaimResources(t, claim, rewardID)
	stored, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil || stored.GetManagerExp() != user.GetManagerExp()+exp || stored.GetManagerLevel() < 80 || !proto.Equal(stored, claim.GetCommonResponse().GetUpdatedData().GetUser()) {
		t.Fatalf("manager EXP/level was not granted and returned: %v %v", stored, err)
	}
	storedPhoto, err := opuser.FindUserPhoto(ctx, testConfig.TestUserSuid, photo.GetPhotoId())
	if err != nil || !storedPhoto.GetAbilities()[0].GetIsAvailable() {
		t.Fatalf("mission level-up did not unlock dedicated photo ability: %v %v", storedPhoto, err)
	}
	returnedPhoto := false
	for _, updated := range claim.GetCommonResponse().GetUpdatedData().GetPhotos() {
		if proto.Equal(updated, storedPhoto) {
			returnedPhoto = true
		}
	}
	if !returnedPhoto {
		t.Fatal("mission reward omitted the unlocked photo post-image")
	}
	get := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	if get.GetUser().GetManagerExp() != stored.GetManagerExp() || get.GetUser().GetManagerLevel() != stored.GetManagerLevel() {
		t.Fatal("User.Get did not retain mission-granted manager EXP")
	}
}

func TestMissionRecurringAndLoveScenario(t *testing.T) {
	setupTestUser(t)
	client := papi.NewMissionClient(testConfig.GrpcConn)
	listed := profileCall(t, client.List, &papi.Empty{})
	daily, dailyRow := selectListedMission(t, listed.GetUserMissions(), penum.MissionType_MissionType_CardLevelUpCount, penum.MissionCategoryType_MissionCategoryType_Daily)
	weekly, weeklyRow := selectListedMission(t, listed.GetUserMissions(), penum.MissionType_MissionType_CardLevelUpCount, penum.MissionCategoryType_MissionCategoryType_Weekly)
	for _, row := range []*ptransaction.UserMission{dailyRow, weeklyRow} {
		row.Progress = row.GetCurrentThreshold()
		row.StatusType = penum.MissionStatusType_MissionStatusType_Received
		row.LastUpdateTime = time.Now().Add(-8 * 24 * time.Hour).UnixMilli()
		if err := opuser.UpsertUserMission(context.Background(), testConfig.TestUserSuid, row); err != nil {
			t.Fatal(err)
		}
	}
	profileCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	first := profileCall(t, client.List, &papi.Empty{})
	second := profileCall(t, client.List, &papi.Empty{})
	for _, id := range []string{daily.GetId(), weekly.GetId()} {
		one, two := findUserMissionByID(first.GetUserMissions(), id), findUserMissionByID(second.GetUserMissions(), id)
		if one.GetProgress() != 0 || one.GetStatusType() != penum.MissionStatusType_MissionStatusType_Progress || !proto.Equal(one, two) {
			t.Fatalf("recurring mission failed reset or reset again within same period: %v %v", one, two)
		}
	}
	_, catalog := registerTestLove(t)
	var loginMaster *pmaster.Mission
	masters, err := opmaster.FindAllMissions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, master := range masters {
		if slices.Contains(catalog.Config.LimitedEventMissionIDs, master.GetEventMissionId()) && master.GetCategoryType() == penum.MissionCategoryType_MissionCategoryType_Daily && master.GetMissionType() == penum.MissionType_MissionType_LoginCount {
			loginMaster = master
			break
		}
	}
	if loginMaster == nil {
		t.Fatal("supported Love event has no daily login mission")
	}
	event := profileCall(t, client.Event, &papi.MissionEventRequest{EventMissionId: loginMaster.GetEventMissionId()})
	if !event.GetInfo().GetUnlocked() || findUserMissionByID(event.GetInfo().GetUserMissions(), loginMaster.GetId()).GetProgress() != 1 {
		t.Fatalf("permanent Love event daily mission is not ready: %v", event.GetInfo())
	}
	claim := profileCall(t, client.Receive, &papi.MissionReceiveRequest{MissionIds: []string{loginMaster.GetId()}})
	assertMissionClaimResources(t, claim, loginMaster.GetProgresses()[0].GetRewardSetId())
	profileReject(t, codes.FailedPrecondition, client.Receive, &papi.MissionReceiveRequest{MissionIds: []string{loginMaster.GetId()}})
	profileReject(t, codes.FailedPrecondition, client.Event, &papi.MissionEventRequest{EventMissionId: "not-an-active-event"})
}

func TestMissionCompletionGraphScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	client := papi.NewMissionClient(testConfig.GrpcConn)
	home := profileCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	rows := profileCall(t, client.List, &papi.Empty{}).GetUserMissions()
	for _, event := range home.GetEventInfo().GetEventMissionInfos() {
		if event.GetUnlocked() {
			rows = append(rows, profileCall(t, client.Event, &papi.MissionEventRequest{EventMissionId: event.GetId()}).GetInfo().GetUserMissions()...)
		}
	}
	ids := make([]string, 0, len(rows))
	byID := map[string]*ptransaction.UserMission{}
	for _, row := range rows {
		ids = append(ids, row.GetMissionId())
		byID[row.GetMissionId()] = row
	}
	masters, err := opmaster.FindMissionsByIds(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	byMaster := map[string]*pmaster.Mission{}
	for _, master := range masters {
		byMaster[master.GetId()] = master
	}
	var parent, grand *ptransaction.UserMission
	var children []*ptransaction.UserMission
	for _, candidate := range masters {
		if candidate.GetMissionType() != penum.MissionType_MissionType_MissionClearCount || byID[candidate.GetId()].GetCurrentThreshold() < 2 {
			continue
		}
		for _, parentID := range candidate.GetTargetIds() {
			parentMaster := byMaster[parentID]
			if parentMaster.GetMissionType() != penum.MissionType_MissionType_MissionClearCount {
				continue
			}
			selected := []*ptransaction.UserMission{}
			for _, childID := range parentMaster.GetTargetIds() {
				child, master := byID[childID], byMaster[childID]
				if child == nil || child.GetProgress() < child.GetCurrentThreshold() {
					continue
				}
				// Only persisted snapshot achievements can cross together on
				// List without inventing an unimplemented live action.
				switch master.GetMissionType() {
				case penum.MissionType_MissionType_ManagerLevelAchievement, penum.MissionType_MissionType_CardLevelAchievement,
					penum.MissionType_MissionType_LoginTotalCount, penum.MissionType_MissionType_ConditionClear:
					selected = append(selected, child)
				}
			}
			if len(selected) >= 2 {
				parent, grand, children = byID[parentID], byID[candidate.GetId()], selected[:2]
				break
			}
		}
		if parent != nil {
			break
		}
	}
	if parent == nil {
		t.Skip("current active master missions have no three-level graph with two simultaneously projectable children")
	}
	// Isolate the actual master graph using only the configured user's rows.
	now := time.Now().UnixMilli()
	for _, row := range rows {
		row.StatusType = penum.MissionStatusType_MissionStatusType_Received
		row.LastUpdateTime = now
	}
	for _, row := range children {
		row.StatusType = penum.MissionStatusType_MissionStatusType_Progress
		row.Progress = 0
	}
	parent.StatusType, parent.Progress = penum.MissionStatusType_MissionStatusType_Progress, parent.GetCurrentThreshold()-1
	grand.StatusType, grand.Progress = penum.MissionStatusType_MissionStatusType_Progress, 0
	if err := opuser.UpsertManyUserMissions(context.Background(), testConfig.TestUserSuid, rows); err != nil {
		t.Fatal(err)
	}
	profileCall(t, client.List, &papi.Empty{})
	stored, err := findUserMission(context.Background(), grand.GetMissionId())
	if err != nil || stored.GetProgress() != 1 {
		t.Fatalf("two sibling completions credited their parent more than once to its parent: %v %v", stored, err)
	}
	profileCall(t, client.List, &papi.Empty{})
	again, err := findUserMission(context.Background(), grand.GetMissionId())
	if err != nil || !proto.Equal(stored, again) {
		t.Fatal("repeated reconciliation duplicated a completed graph edge")
	}
}

func selectListedMission(t *testing.T, rows []*ptransaction.UserMission, typ penum.MissionType, category penum.MissionCategoryType) (*pmaster.Mission, *ptransaction.UserMission) {
	t.Helper()
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.GetMissionId())
	}
	masters, err := opmaster.FindMissionsByIds(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	for _, master := range masters {
		if master.GetMissionType() == typ && master.GetCategoryType() == category {
			return master, findUserMissionByID(rows, master.GetId())
		}
	}
	t.Fatalf("no active master mission for type %v category %v", typ, category)
	return nil, nil
}

func assertMissionClaimResources(t *testing.T, response *papi.MissionReceiveResponse, rewardID string) {
	t.Helper()
	resources, err := resolveCatalogRewards(context.Background(), rewardID)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range resources {
		var amount int64
		for _, result := range response.GetResults() {
			if result.GetResourceType() == resource.GetResourceType() && result.GetResourceId() == resource.GetResourceId() {
				amount += result.GetAmountI64()
				if result.GetAfterAmount()-result.GetBeforeAmount() != result.GetAmountI64() {
					t.Fatalf("reward before/after mismatch: %v", result)
				}
			}
		}
		if amount != int64(resource.GetAmount()) {
			t.Fatalf("mission reward did not match current master: %v, results %v", resource, response.GetResults())
		}
	}
}
