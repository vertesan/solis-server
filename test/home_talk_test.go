package test

import (
	"context"
	"testing"

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

func TestHomeReadTalkScenario(t *testing.T) {
	setupTestUser(t)
	dbCtx := context.Background()
	characters, err := opuser.FindAllUserCharacters(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	owned := make(map[string]*ptransaction.UserCharacter, len(characters))
	for _, character := range characters {
		owned[character.GetCharacterId()] = character
	}
	masters, err := opmaster.FindAllHomeTalks(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	var talk, locked *pmaster.HomeTalk
	for _, master := range masters {
		if owned[master.GetCharacterId()] == nil || master.GetCardId() != "" || master.GetReleaseDate() != 0 {
			continue
		}
		if master.GetConditionId() == "" && master.GetReliability() > 0 {
			talk = master
		} else if locked == nil && master.GetConditionId() != "" {
			condition, err := opmaster.FindCondition(dbCtx, master.GetConditionId())
			if err != nil {
				t.Fatal(err)
			}
			if len(condition.GetSettings()) == 1 {
				requirement := condition.GetSettings()[0].GetReliability()
				if requirement != nil && requirement.GetCharacterId() == master.GetCharacterId() &&
					requirement.GetReliabilityGte() > owned[master.GetCharacterId()].GetReliability() {
					locked = master
				}
			}
		}
	}
	if talk == nil || locked == nil {
		t.Fatal("master data lacks owned characters' unlocked and locked home talks")
	}
	before := owned[talk.GetCharacterId()]
	expectedReliability := before.GetReliability() + talk.GetReliability()

	// An active reliability mission must observe the reward, just as in the capture.
	missionMasters, err := opmaster.FindAllMissions(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	var missionID string
	for _, master := range missionMasters {
		if master.GetMissionType() != penum.MissionType_MissionType_ReliabilityAchievement ||
			len(master.GetTargetIds()) != 1 || master.GetTargetIds()[0] != talk.GetCharacterId() ||
			len(master.GetProgresses()) == 0 || master.GetProgresses()[0].GetThreshold() <= int64(expectedReliability) {
			continue
		}
		missionID = master.GetId()
		if err := opuser.UpsertUserMission(dbCtx, testConfig.TestUserSuid, &ptransaction.UserMission{
			MissionId: missionID, CurrentThreshold: master.GetProgresses()[0].GetThreshold(),
			StatusType: penum.MissionStatusType_MissionStatusType_Progress,
		}); err != nil {
			t.Fatal(err)
		}
		break
	}
	if missionID == "" {
		t.Fatal("no reliability mission for selected home talk character")
	}
	var talkMissionID string
	for _, master := range missionMasters {
		if master.GetMissionType() != penum.MissionType_MissionType_CharacterHomeTalkReadAchievement ||
			len(master.GetTargetIds()) != 1 || master.GetTargetIds()[0] != talk.GetCharacterId() || len(master.GetProgresses()) == 0 {
			continue
		}
		talkMissionID = master.GetId()
		if err := opuser.UpsertUserMission(dbCtx, testConfig.TestUserSuid, &ptransaction.UserMission{
			MissionId: talkMissionID, CurrentThreshold: master.GetProgresses()[0].GetThreshold(),
			StatusType: penum.MissionStatusType_MissionStatusType_Progress,
		}); err != nil {
			t.Fatal(err)
		}
		break
	}
	if talkMissionID == "" {
		t.Fatal("no home talk read mission for selected character")
	}
	readTalk := func(id string) (*papi.HomeReadTalkResponse, error) {
		t.Helper()
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		defer cancel()
		return papi.NewHomeClient(testConfig.GrpcConn).ReadTalk(ctx, &papi.HomeReadTalkRequest{HomeTalkId: id})
	}
	if _, err := readTalk(locked.GetHomeTalkId()); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("locked home talk returned %v", err)
	}
	if _, err := readTalk("home-talk-does-not-exist"); status.Code(err) != codes.NotFound {
		t.Fatalf("unknown home talk returned %v", err)
	}
	response, err := readTalk(talk.GetHomeTalkId())
	if err != nil {
		t.Fatal(err)
	}
	updated := response.GetCommonResponse().GetUpdatedData()
	if !response.GetIsReliabilityUp() || len(updated.GetCharacters()) != 1 ||
		updated.GetCharacters()[0].GetReliability() != expectedReliability ||
		updated.GetCharacters()[0].GetLastHomeTalkFirstReadTime() <= before.GetLastHomeTalkFirstReadTime() ||
		len(updated.GetHomeTalks()) != 1 || updated.GetHomeTalks()[0].GetHomeTalkId() != talk.GetHomeTalkId() {
		t.Fatalf("unexpected first-read response: %v", response)
	}
	foundMission := false
	for _, mission := range updated.GetMissions() {
		if mission.GetMissionId() == missionID && mission.GetProgress() == int64(expectedReliability) {
			foundMission = true
		}
	}
	if !foundMission {
		t.Fatal("home talk reward did not advance reliability mission")
	}
	foundMission = false
	for _, mission := range updated.GetMissions() {
		if mission.GetMissionId() == talkMissionID && mission.GetProgress() == 1 {
			foundMission = true
		}
	}
	if !foundMission {
		t.Fatal("first home talk read did not advance its company mission")
	}
	firstCharacter := updated.GetCharacters()[0]
	if replay, err := readTalk(talk.GetHomeTalkId()); err != nil || replay.GetIsReliabilityUp() {
		t.Fatalf("rereading a home talk must not award reliability again: %v, %v", replay, err)
	}
	stored, err := opuser.FindUserCharacter(dbCtx, testConfig.TestUserSuid, talk.GetCharacterId())
	if err != nil || !proto.Equal(stored, firstCharacter) {
		t.Fatalf("reread changed character state: %v, %v", stored, err)
	}
	missions, err := opuser.FindAllUserMissions(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, mission := range missions {
		if mission.GetMissionId() == talkMissionID && mission.GetProgress() != 1 {
			t.Fatalf("reread advanced home talk mission again: %v", mission)
		}
	}
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	user, err := papi.NewUserClient(testConfig.GrpcConn).Get(ctx, &papi.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if len(user.GetHomeTalks()) != 1 || user.GetHomeTalks()[0].GetHomeTalkId() != talk.GetHomeTalkId() {
		t.Fatalf("User.Get did not preserve home talk history: %v", user.GetHomeTalks())
	}
	foundCharacter := false
	for _, character := range user.GetCharacters() {
		if character.GetCharacterId() == talk.GetCharacterId() {
			foundCharacter = character.GetReliability() == expectedReliability &&
				character.GetLastHomeTalkFirstReadTime() == firstCharacter.GetLastHomeTalkFirstReadTime()
		}
	}
	if !foundCharacter {
		t.Fatal("User.Get did not preserve home talk character rewards")
	}
	// Company visits recover achievement progress for history recorded before
	// the mission became available, without granting reliability a second time.
	for _, mission := range missions {
		if mission.GetMissionId() != talkMissionID {
			continue
		}
		mission.Progress = 0
		if err := opuser.UpsertUserMission(dbCtx, testConfig.TestUserSuid, mission); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := papi.NewCompanyClient(testConfig.GrpcConn).EnjoyCharacterTop(ctx,
		&papi.CompanyEnjoyCharacterTopRequest{CharacterId: talk.GetCharacterId()}); err != nil {
		t.Fatal(err)
	}
	missions, err = opuser.FindAllUserMissions(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	foundMission = false
	for _, mission := range missions {
		if mission.GetMissionId() == talkMissionID && mission.GetProgress() == 1 {
			foundMission = true
		}
	}
	if !foundMission {
		t.Fatal("Company did not reconcile recorded home talk history")
	}
}
