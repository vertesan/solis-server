package test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

func TestDreamMonthlyPeriodScenario(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	for _, boundary := range []time.Time{
		time.Date(2026, 9, 15, 12, 0, 0, 0, jst),
		time.Date(2027, 1, 15, 12, 0, 0, 0, jst),
		time.Date(2028, 3, 15, 12, 0, 0, 0, jst),
	} {
		before, end := dream.MonthlyPeriod(boundary.UnixMilli() - 1)
		if before != boundary.AddDate(0, -1, 0).UnixMilli() || end != boundary.UnixMilli() {
			t.Fatalf("wrong pre-boundary period at %v: %d %d", boundary, before, end)
		}
		start, end := dream.MonthlyPeriod(boundary.UnixMilli())
		if start != boundary.UnixMilli() || end != boundary.AddDate(0, 1, 0).UnixMilli() {
			t.Fatalf("wrong post-boundary period at %v: %d %d", boundary, start, end)
		}
	}
}

func TestDreamMonthlyReconciliationScenario(t *testing.T) {
	setupTestUser(t)
	ctx, suid := context.Background(), testConfig.TestUserSuid
	// Move beyond the last master month to cover permanent template reuse too.
	now := time.Now().AddDate(2, 0, 0).UnixMilli()
	var first *dream.MonthlyState
	reconcile := func(at int64) *papi.UpdatedData {
		t.Helper()
		updated := &papi.UpdatedData{}
		if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
			var err error
			first, err = dream.ReconcileMonthly(ctx, suid, at, updated)
			return nil, err
		}); err != nil {
			t.Fatal(err)
		}
		return updated
	}
	updated := reconcile(now)
	if len(updated.GetMissions()) != len(first.Missions) || len(first.Missions) == 0 {
		t.Fatalf("missing monthly missions: %v", updated)
	}
	giftID := fmt.Sprintf("dream-monthly-%d", first.StartTime)
	gift, err := opuser.FindUserGift(ctx, suid, giftID)
	if err != nil || gift.GetResourceId() != dream.ManualItemID || gift.GetAmount() != 5 || gift.GetLimitTime() != 0 {
		t.Fatalf("invalid monthly gift: %v %v", gift, err)
	}
	giftNotification, err := opuser.FindUserNotification(ctx, suid, penum.NotificationType_NotificationType_Gift)
	if err != nil || !giftNotification.GetValid() {
		t.Fatalf("monthly gift did not light mailbox notification: %v %v", giftNotification, err)
	}
	mission := proto.Clone(updated.GetMissions()[0]).(*ptransaction.UserMission)
	mission.Progress = mission.GetCurrentThreshold()
	mission.StatusType = penum.MissionStatusType_MissionStatusType_Received
	if err := opuser.UpsertUserMission(ctx, suid, mission); err != nil {
		t.Fatal(err)
	}
	lifetime := &ptransaction.UserMission{MissionId: "ms-normal-dream_clear_difficulty_level", CurrentThreshold: 20, Progress: 100, StatusType: penum.MissionStatusType_MissionStatusType_NotReceived, LastUpdateTime: now}
	if err := opuser.UpsertUserMission(ctx, suid, lifetime); err != nil {
		t.Fatal(err)
	}
	state, err := opuser.FindUserDreamState(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	state.QuestClearCount, state.MaxDifficulty = 42, 250
	if err := opuser.UpsertUserDreamState(ctx, state); err != nil {
		t.Fatal(err)
	}
	if err := opuser.DeleteUserGift(ctx, suid, giftID); err != nil {
		t.Fatal(err)
	}
	// Simulate the mailbox indicator cleared after collecting that gift.
	giftNotification.Valid = false
	if err := opuser.UpsertUserNotification(ctx, suid, giftNotification); err != nil {
		t.Fatal(err)
	}
	updated = reconcile(now + 1)
	if len(updated.GetMissions()) != 0 {
		t.Fatal("same-period reconciliation reset missions")
	}
	if len(updated.GetNotifications()) != 0 {
		t.Fatal("same-period reconciliation rewrote mailbox notification")
	}
	storedNotification, err := opuser.FindUserNotification(ctx, suid, penum.NotificationType_NotificationType_Gift)
	if err != nil || !proto.Equal(storedNotification, giftNotification) {
		t.Fatalf("collected gift notification was relit: %v %v", storedNotification, err)
	}
	if _, err := opuser.FindUserGift(ctx, suid, giftID); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("already collected gift was reissued: %v", err)
	}
	stored, err := opuser.FindUserMission(ctx, suid, mission.GetMissionId())
	if err != nil || !proto.Equal(stored, mission) {
		t.Fatalf("same-period receipt changed: %v %v", stored, err)
	}
	nextBoundary := first.EndTime
	updated = reconcile(nextBoundary)
	if len(updated.GetMissions()) != len(first.Missions) {
		t.Fatal("monthly boundary did not reset missions")
	}
	for _, row := range updated.GetMissions() {
		if row.GetProgress() != 0 || row.GetStatusType() != penum.MissionStatusType_MissionStatusType_Progress {
			t.Fatalf("mission did not reset: %v", row)
		}
	}
	stored, err = opuser.FindUserMission(ctx, suid, lifetime.GetMissionId())
	if err != nil || !proto.Equal(stored, lifetime) {
		t.Fatalf("lifetime mission was reset: %v %v", stored, err)
	}
	state, err = opuser.FindUserDreamState(ctx, suid)
	if err != nil || state.QuestClearCount != 42 || state.MaxDifficulty != 250 {
		t.Fatalf("lifetime counters reset: %v %v", state, err)
	}
	if gift, err := opuser.FindUserGift(ctx, suid, fmt.Sprintf("dream-monthly-%d", nextBoundary)); err != nil || gift.GetAmount() != 5 {
		t.Fatalf("new month gift missing: %v %v", gift, err)
	}
	storedNotification, err = opuser.FindUserNotification(ctx, suid, penum.NotificationType_NotificationType_Gift)
	if err != nil || !storedNotification.GetValid() {
		t.Fatalf("new month gift did not restore mailbox notification: %v %v", storedNotification, err)
	}
	// A stale clock/request cannot rewind monthly state or grant an old period.
	if updated := reconcile(now); len(updated.GetMissions()) != 0 {
		t.Fatal("stale reconciliation rewound monthly state")
	}
}

func TestDreamHomeMissionIntegrationScenario(t *testing.T) {
	setupTestUser(t)
	home := profileCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	info := home.GetEventInfo().GetDreamInfo()
	if info == nil || info.GetIsUnlocked() || info.GetEndTime() != 0 || info.GetGenerateAreaItemId() != dream.ManualItemID {
		t.Fatalf("Dream event is not permanent: %v", info)
	}
	promoteTestUser(t)
	home = profileCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	info = home.GetEventInfo().GetDreamInfo()
	if !info.GetIsUnlocked() {
		t.Fatal("god user has not unlocked Dream event")
	}
	event := profileCall(t, papi.NewMissionClient(testConfig.GrpcConn).Event, &papi.MissionEventRequest{EventMissionId: info.GetEventMissionId()})
	if !event.GetInfo().GetUnlocked() || len(event.GetInfo().GetUserMissions()) == 0 {
		t.Fatalf("monthly missions not available: %v", event)
	}
	get := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	for _, row := range event.GetInfo().GetUserMissions() {
		if !proto.Equal(row, findUserMissionByID(get.GetMissions(), row.GetMissionId())) {
			t.Fatalf("User.Get omitted monthly mission %s", row.GetMissionId())
		}
	}
	for _, row := range home.GetEventInfo().GetEventMissionInfos() {
		if row.GetId() == info.GetEventMissionId() {
			t.Fatal("Dream mission was duplicated as a campaign entrance")
		}
	}
}

func TestDreamGradeMissionIsolationScenario(t *testing.T) {
	setupTestUser(t)
	ctx, suid := context.Background(), testConfig.TestUserSuid
	base, material := dreamTestPhotos(t, ctx, suid)
	abilities, err := dream.LoadAbilities(ctx, []*ptransaction.UserPhoto{base, material})
	if err != nil {
		t.Fatal(err)
	}
	for _, photo := range []*ptransaction.UserPhoto{base, material} {
		ids := make([]string, 0, len(photo.GetAbilities()))
		for _, ability := range photo.GetAbilities() {
			ids = append(ids, ability.GetPhotoAbilityId())
		}
		photo.Level = 150
		photo.Abilities = buildTestPhotoAbilities(t, &pmaster.PhotoAbilitySet{PhotoAbilityIds: ids}, abilities, photo.GetLevel())
		if err := opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
			t.Fatal(err)
		}
	}
	if err := opuser.UpsertUserItem(ctx, suid, &ptransaction.UserItem{ItemId: dream.ManualItemID, Amount: 100}); err != nil {
		t.Fatal(err)
	}
	masters, err := opmaster.FindAllMissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var unrelated *ptransaction.UserMission
	for _, master := range masters {
		if master.GetMissionType() == penum.MissionType_MissionType_ConditionClear && len(master.GetTargetIds()) == 0 && len(master.GetProgresses()) > 0 {
			unrelated = &ptransaction.UserMission{MissionId: master.GetId(), CurrentThreshold: master.GetProgresses()[0].GetThreshold(), StatusType: penum.MissionStatusType_MissionStatusType_Progress, LastUpdateTime: time.Now().UnixMilli()}
			break
		}
	}
	if unrelated == nil {
		t.Fatal("master has no targetless condition mission for isolation scenario")
	}
	if err := opuser.UpsertUserMission(ctx, suid, unrelated); err != nil {
		t.Fatal(err)
	}
	generated := profileCall(t, papi.NewDreamClient(testConfig.GrpcConn).GenerateArea, &papi.DreamGenerateAreaRequest{BasePhotoId: base.GetPhotoId(), MaterialPhotoId: material.GetPhotoId()})
	if generated.GetDreamUserPhoto().GetAreaRankType() < penum.DreamAreaRankType_DreamAreaRankType_A {
		t.Fatal("fixture did not generate grade A")
	}
	stored, err := opuser.FindUserMission(ctx, suid, unrelated.GetMissionId())
	if err != nil || !proto.Equal(stored, unrelated) {
		t.Fatalf("Dream grade progressed an unrelated condition mission: %v %v", stored, err)
	}
	monthly, err := dream.MonthlyCatalog(ctx, time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	var gradeID string
	for _, master := range monthly.Missions {
		if master.GetMissionType() == penum.MissionType_MissionType_ConditionClear {
			gradeID = master.GetId()
		}
	}
	if gradeID == "" {
		t.Fatal("monthly grade mission is absent")
	}
	grade, err := opuser.FindUserMission(ctx, suid, gradeID)
	if err != nil || grade.GetProgress() != 1 || grade.GetStatusType() != penum.MissionStatusType_MissionStatusType_NotReceived {
		t.Fatalf("monthly grade mission did not complete: %v %v", grade, err)
	}
	profileCall(t, papi.NewMissionClient(testConfig.GrpcConn).Receive, &papi.MissionReceiveRequest{MissionIds: []string{gradeID}})
	profileCall(t, papi.NewDreamClient(testConfig.GrpcConn).Top, &papi.Empty{})
	grade, err = opuser.FindUserMission(ctx, suid, gradeID)
	if err != nil || grade.GetStatusType() != penum.MissionStatusType_MissionStatusType_Received {
		t.Fatalf("monthly grade receipt was reset: %v %v", grade, err)
	}
}
