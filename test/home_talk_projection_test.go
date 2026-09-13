package test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/protobuf/proto"
)

func TestHomeTalkMultiCharacterProjectionScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	characters, err := opuser.FindAllUserCharacters(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	talks, err := opmaster.FindAllHomeTalks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	masters, err := opmaster.FindAllMissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(masters, func(i, j int) bool { return masters[i].GetId() < masters[j].GetId() })
	type fixture struct {
		mission *pmaster.Mission
		talks   []*pmaster.HomeTalk
		unread  *pmaster.HomeTalk
		count   int64
	}
	fixtures := make([]fixture, 0, 2)
	for _, character := range characters {
		var candidate fixture
		for _, master := range masters {
			if master.GetMissionType() == penum.MissionType_MissionType_CharacterHomeTalkReadAchievement &&
				len(master.GetTargetIds()) == 1 && master.GetTargetIds()[0] == character.GetCharacterId() &&
				len(master.GetProgresses()) == 1 && master.GetProgresses()[0].GetThreshold() > 1 {
				candidate.mission = master
				break
			}
		}
		if candidate.mission == nil {
			continue
		}
		for _, talk := range talks {
			if talk.GetCharacterId() != character.GetCharacterId() {
				continue
			}
			if candidate.unread == nil && talk.GetConditionId() == "" && talk.GetCardId() == "" && talk.GetReleaseDate() <= time.Now().UnixMilli() {
				candidate.unread = talk
				continue
			}
			candidate.talks = append(candidate.talks, talk)
		}
		threshold := candidate.mission.GetProgresses()[0].GetThreshold()
		if candidate.unread == nil || int64(len(candidate.talks)) < threshold {
			continue
		}
		// One character has already completed its achievement; the other is
		// one distinct conversation short. Their counts must not be combined.
		candidate.count = threshold - int64(len(fixtures))
		fixtures = append(fixtures, candidate)
		if len(fixtures) == 2 {
			break
		}
	}
	if len(fixtures) != 2 {
		t.Fatal("master data lacks two owned characters with home talk achievements")
	}
	for _, fixture := range fixtures {
		// These first-read records predate Company availability. Home must
		// recover the achievement without granting reliability again.
		for _, talk := range fixture.talks[:fixture.count] {
			if err := opuser.UpsertUserHomeTalk(ctx, testConfig.TestUserSuid, &ptransaction.UserHomeTalk{HomeTalkId: talk.GetHomeTalkId()}); err != nil {
				t.Fatal(err)
			}
		}
		if err := opuser.UpsertUserMission(ctx, testConfig.TestUserSuid, &ptransaction.UserMission{
			MissionId: fixture.mission.GetId(), CurrentThreshold: fixture.mission.GetProgresses()[0].GetThreshold(),
			StatusType: penum.MissionStatusType_MissionStatusType_Progress,
		}); err != nil {
			t.Fatal(err)
		}
	}
	condition, err := opmaster.FindCondition(ctx, "cd-function_lock-company")
	if err != nil || len(condition.GetSettings()) != 1 || condition.GetSettings()[0].GetClearArea().GetAreaId() == "" {
		t.Fatalf("Company unlock condition changed: %v %v", condition, err)
	}
	area, err := opmaster.FindArea(ctx, condition.GetSettings()[0].GetClearArea().GetAreaId())
	if err != nil {
		t.Fatal(err)
	}
	if err := opuser.UnlockUserAreasForGod(ctx, testConfig.TestUserSuid, []*ptransaction.UserArea{{
		AreaId: area.GetId(), ClearQuestCount: int32(len(area.GetQuestIds())), LastClearedTime: time.Now().UnixMilli(),
	}}); err != nil {
		t.Fatal(err)
	}
	home := papi.NewHomeClient(testConfig.GrpcConn)
	entered := profileCall(t, home.Enter, &papi.HomeEnterRequest{})
	if !entered.GetCompanyInfo().GetIsReceivableEnjoyRewards() {
		t.Fatal("Home omitted Company reward notification for a completed conversation achievement")
	}
	projected := make([]*ptransaction.UserMission, len(fixtures))
	for i, fixture := range fixtures {
		row, err := opuser.FindUserMission(ctx, testConfig.TestUserSuid, fixture.mission.GetId())
		wantStatus := penum.MissionStatusType_MissionStatusType_Progress
		if i == 0 {
			wantStatus = penum.MissionStatusType_MissionStatusType_NotReceived
		}
		if err != nil || row.GetProgress() != fixture.count || row.GetStatusType() != wantStatus {
			t.Fatalf("Home projected wrong count/status for %s: %v %v", fixture.mission.GetId(), row, err)
		}
		projected[i] = row
	}
	profileCall(t, home.Enter, &papi.HomeEnterRequest{})
	for i, fixture := range fixtures {
		profileCall(t, papi.NewCompanyClient(testConfig.GrpcConn).EnjoyCharacterTop,
			&papi.CompanyEnjoyCharacterTopRequest{CharacterId: fixture.mission.GetTargetIds()[0]})
		row, err := opuser.FindUserMission(ctx, testConfig.TestUserSuid, fixture.mission.GetId())
		if err != nil || !proto.Equal(row, projected[i]) {
			t.Fatalf("repeated Home/Company visit changed first-read progress: %v %v", row, err)
		}
	}
	for _, before := range characters {
		after, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, before.GetCharacterId())
		if err != nil || after.GetReliability() != before.GetReliability() || after.GetLastHomeTalkFirstReadTime() != before.GetLastHomeTalkFirstReadTime() {
			t.Fatal("history projection granted a home talk character reward again")
		}
	}
	// An actual new conversation still completes the second character's
	// achievement after the optimized Home projection has run.
	response := profileCall(t, home.ReadTalk, &papi.HomeReadTalkRequest{HomeTalkId: fixtures[1].unread.GetHomeTalkId()})
	row := findUserMissionByID(response.GetCommonResponse().GetUpdatedData().GetMissions(), fixtures[1].mission.GetId())
	if row.GetProgress() != fixtures[1].count+1 || row.GetStatusType() != penum.MissionStatusType_MissionStatusType_NotReceived {
		t.Fatalf("new conversation did not complete its character achievement: %v", row)
	}
	profileCall(t, home.Enter, &papi.HomeEnterRequest{})
	stored, err := opuser.FindUserMission(ctx, testConfig.TestUserSuid, fixtures[1].mission.GetId())
	if err != nil || !proto.Equal(stored, row) {
		t.Fatalf("Home changed an already projected new conversation: %v %v", stored, err)
	}
}
