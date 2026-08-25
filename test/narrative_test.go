package test

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	messagepath "github.com/vertesan/solis-server/internal/makino/message"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/http/admin"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestCostumeScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	ctx := context.Background()
	costume, hair := findWearableCostumePair(t)

	userCostume, err := opuser.FindUserCostume(ctx, testConfig.TestUserSuid, costume.Id)
	if err != nil {
		t.Fatal(err)
	}
	userHair, err := opuser.FindUserHair(ctx, testConfig.TestUserSuid, hair.Id)
	if err != nil {
		t.Fatal(err)
	}
	obtainedTime := userCostume.ObtainedTime
	userCostume.Checked, userHair.Checked = true, true
	if err := opuser.UpsertUserCostume(ctx, testConfig.TestUserSuid, userCostume); err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserHair(ctx, testConfig.TestUserSuid, userHair); err != nil {
		t.Fatal(err)
	}

	client := papi.NewCostumeClient(testConfig.GrpcConn)
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	liveResponse, err := client.SetLiveCostume(grpcCtx, &papi.CostumeLiveSetRequest{CostumeId: costume.Id, HairId: hair.Id})
	cancel()
	if err != nil {
		t.Fatalf("Costume.SetLiveCostume failed: %v", err)
	}
	updated := liveResponse.GetCommonResponse().GetUpdatedData()
	if len(updated.GetCharacters()) != 1 || updated.Characters[0].LiveCostumeId != costume.Id || updated.Characters[0].LiveHairId != hair.Id {
		t.Fatalf("Costume.SetLiveCostume returned the wrong character post-image: %v", liveResponse)
	}
	assertWardrobeChecked(t, costume.Id, hair.Id, false, true)

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	checkResponse, err := client.CheckCostume(grpcCtx, &papi.CostumeCheckRequest{CostumeId: costume.Id})
	cancel()
	if err != nil {
		t.Fatalf("Costume.CheckCostume failed: %v", err)
	}
	checkedCostumes := checkResponse.GetCommonResponse().GetUpdatedData().GetCostumes()
	if len(checkedCostumes) != 1 || checkedCostumes[0].GetCostumeId() != costume.Id ||
		!checkedCostumes[0].GetChecked() || checkedCostumes[0].GetObtainedTime() != obtainedTime {
		t.Fatalf("Costume.CheckCostume returned the wrong costume post-image: %v", checkResponse)
	}
	assertWardrobeChecked(t, costume.Id, hair.Id, true, true)

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	repeatCheck, err := client.CheckCostume(grpcCtx, &papi.CostumeCheckRequest{CostumeId: costume.Id})
	cancel()
	if err != nil {
		t.Fatalf("repeated Costume.CheckCostume failed: %v", err)
	}
	if repeatCheck.GetCommonResponse() == nil || len(repeatCheck.GetCommonResponse().GetUpdatedData().GetCostumes()) != 0 {
		t.Fatalf("repeated Costume.CheckCostume returned a redundant update: %v", repeatCheck)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SetCostume(grpcCtx, &papi.CostumeSetRequest{CostumeId: costume.Id, HairId: hair.Id})
	cancel()
	if err != nil {
		t.Fatalf("Costume.SetCostume failed: %v", err)
	}
	character, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, costume.CharacterId)
	if err != nil {
		t.Fatal(err)
	}
	if character.FavoriteCostumeId != costume.Id || character.FavoriteHairId != hair.Id {
		t.Fatalf("favorite wardrobe was not persisted: %v", character)
	}
	assertWardrobeChecked(t, costume.Id, hair.Id, true, false)

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CheckBulk(grpcCtx, &papi.CostumeCheckBulkRequest{CostumeIds: []string{costume.Id}, HairIds: []string{hair.Id}})
	cancel()
	if err != nil {
		t.Fatalf("Costume.CheckBulk failed: %v", err)
	}
	assertWardrobeChecked(t, costume.Id, hair.Id, true, true)
}

func TestStoryScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	client := papi.NewStoryClient(testConfig.GrpcConn)
	readStoryId := findRewardedUnreadStory(t)

	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	readResponse, err := client.Read(grpcCtx, &papi.StoryReadRequest{StoryId: readStoryId})
	cancel()
	if err != nil {
		t.Fatalf("Story.Read failed: %v", err)
	}
	if len(readResponse.GetRewards()) == 0 {
		t.Fatal("Story.Read did not grant its master reward")
	}
	assertStoryRead(t, readStoryId)

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	repeatResponse, err := client.Read(grpcCtx, &papi.StoryReadRequest{StoryId: readStoryId})
	cancel()
	if err != nil {
		t.Fatalf("repeated Story.Read failed: %v", err)
	}
	if len(repeatResponse.GetRewards()) != 0 {
		t.Fatalf("repeated Story.Read granted duplicate rewards: %v", repeatResponse.Rewards)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	listResponse, err := client.ListEvent(grpcCtx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("Story.ListEvent failed: %v", err)
	}
	eventId, eventEpisode, eventStoryId := findUnlockedEventEpisode(t, listResponse.Infos, readStoryId)
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ReadEvent(grpcCtx, &papi.StoryReadEventRequest{EventStoryId: eventId, Episode: eventEpisode})
	cancel()
	if err != nil {
		t.Fatalf("Story.ReadEvent failed: %v", err)
	}
	assertStoryRead(t, eventStoryId)

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	extraList, err := client.ListExtra(grpcCtx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("Story.ListExtra failed: %v", err)
	}
	if extraList.GetCommonResponse() == nil {
		t.Fatalf("Story.ListExtra returned no CommonResponse: %v", extraList)
	}
	storedExtraInfos, err := opuser.FindAllUserExtraStoryPartInfos(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	sortExtraStoryPartInfos(storedExtraInfos)
	if len(extraList.GetInfos()) != len(storedExtraInfos) {
		t.Fatalf("Story.ListExtra returned %d parts, want %d", len(extraList.GetInfos()), len(storedExtraInfos))
	}
	for index := range storedExtraInfos {
		if !proto.Equal(extraList.GetInfos()[index], storedExtraInfos[index]) {
			t.Fatalf("Story.ListExtra part %d differs from persisted state: got=%v want=%v", index, extraList.GetInfos()[index], storedExtraInfos[index])
		}
	}

	extraId, extraEpisode, extraStoryId := findUnlockedExtraEpisode(t, extraList.GetInfos(), readStoryId, eventStoryId)
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ReadExtra(grpcCtx, &papi.StoryReadExtraRequest{ExtraStoryId: extraId, Episode: extraEpisode})
	cancel()
	if err != nil {
		t.Fatalf("Story.ReadExtra failed: %v", err)
	}
	assertStoryRead(t, extraStoryId)

	characterId, number, companyStoryId := findCompanyEnjoyStory(t, readStoryId, eventStoryId, extraStoryId)
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ReadCharacterCompanyEnjoyStory(grpcCtx, &papi.StoryReadCharacterCompanyEnjoyStoryRequest{
		CharacterId: characterId, Number: number, IsLast: true,
	})
	cancel()
	if err != nil {
		t.Fatalf("Story.ReadCharacterCompanyEnjoyStory failed: %v", err)
	}
	assertStoryRead(t, companyStoryId)

}

func TestDiaryScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	testDiaryReadFlow(t)
}

func TestInitialMessageScenario(t *testing.T) {
	setupTestUser(t)
	userClient := papi.NewUserClient(testConfig.GrpcConn)
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	userData, err := userClient.Get(grpcCtx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("User.Get for the initial message failed: %v", err)
	}
	const tutorialMessageID = "message-skr-tutorial-001"
	tutorial := findUserMessageByID(userData.GetMessages(), tutorialMessageID)
	var tutorialSchedule *ptransaction.UserMessageSchedule
	for _, schedule := range userData.GetMessageSchedules() {
		if schedule.GetMessageId() == tutorialMessageID {
			tutorialSchedule = schedule
			break
		}
	}
	if tutorial == nil || tutorialSchedule == nil ||
		tutorial.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Received ||
		tutorial.GetArrivedTime() <= 0 || tutorialSchedule.GetScheduledTime() != tutorial.GetArrivedTime() ||
		len(tutorial.GetSelectMessageDetailIds()) != 0 {
		t.Fatalf("fresh tutorial message state is invalid: message=%v schedule=%v", tutorial, tutorialSchedule)
	}

	master, err := opmaster.FindMessage(context.Background(), tutorialMessageID)
	if err != nil {
		t.Fatal(err)
	}
	messageClient := papi.NewMessageClient(testConfig.GrpcConn)
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	timeline, err := messageClient.Timeline(grpcCtx, &papi.MessageTimelineRequest{
		MessageGroupId: master.GetMessageGroupId(),
	})
	cancel()
	if err != nil {
		t.Fatalf("Message.Timeline for the initial message failed: %v", err)
	}
	info := findTimelineInfo(t, timeline.GetInfos(), tutorialMessageID)
	if info.GetFinished() || info.GetLastMessageDetailId() != "" || info.GetLastMessageDetailTime() != 0 ||
		len(info.GetSelectMessageDetailIds()) != 0 {
		t.Fatalf("fresh tutorial timeline is not unread: %v", info)
	}
	selections, terminal, err := messagepath.FirstPath(master)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(selections, []string{"2"}) {
		t.Fatalf("tutorial first selection = %v, want [2]", selections)
	}
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	finish, err := messageClient.Finish(grpcCtx, &papi.MessageFinishRequest{
		MessageGroupId:         master.GetMessageGroupId(),
		TimelineId:             info.GetTimelineId(),
		SelectMessageDetailIds: selections,
	})
	cancel()
	if err != nil {
		t.Fatalf("Message.Finish for the initial message failed: %v", err)
	}
	finished := findUserMessageByID(finish.GetCommonResponse().GetUpdatedData().GetMessages(), tutorialMessageID)
	if finished == nil || finished.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Finished ||
		!slices.Equal(finished.GetSelectMessageDetailIds(), selections) {
		t.Fatalf("initial message did not finish with its required choice: %v", finish)
	}
	progress, err := opuser.FindUserMessageProgress(context.Background(), testConfig.TestUserSuid, tutorialMessageID)
	if err != nil || progress.LastMessageDetailId != terminal.GetMessageDetailId() || progress.LastMessageDetailTime <= 0 {
		t.Fatalf("initial message progress was not persisted: progress=%v err=%v", progress, err)
	}
	if _, err := opuser.FindUserMessageSchedule(context.Background(), testConfig.TestUserSuid, tutorialMessageID); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("initial message schedule was not deleted: %v", err)
	}
}

func TestMessageScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	ctx := context.Background()
	master, terminal, selections, firstDetail := findReliabilityMessage(t)
	alternateSelections, ok := findAlternateMessagePath(master, terminal.GetMessageDetailId(), selections)
	if !ok {
		t.Fatal("selected reliability message has no alternate path")
	}
	rewardCharacterID := terminal.GetCharacterId()
	if rewardCharacterID == "" {
		rewardCharacterID = master.GetCharacterId()
	}
	message, err := opuser.FindUserMessage(ctx, testConfig.TestUserSuid, master.Id)
	if err != nil {
		t.Fatal(err)
	}
	message.MessageStatusType = penum.MessageStatusType_MessageStatusType_Scheduled
	message.SelectMessageDetailIds = []string{}
	message.ArrivedTime = 0
	if err := opuser.UpsertUserMessage(ctx, testConfig.TestUserSuid, message); err != nil {
		t.Fatal(err)
	}
	if err := opuser.DeleteUserMessageProgress(ctx, testConfig.TestUserSuid, master.Id); err != nil {
		t.Fatal(err)
	}
	futureSchedule := &ptransaction.UserMessageSchedule{
		MessageId: master.Id, ScheduledTime: time.Now().Add(time.Minute).UnixMilli(),
	}
	if err := opuser.UpsertUserMessageSchedule(ctx, testConfig.TestUserSuid, futureSchedule); err != nil {
		t.Fatal(err)
	}
	for _, telephoneID := range selectedMessageTelephoneIDs(master, selections) {
		if err := opuser.UpsertUserTelephone(ctx, testConfig.TestUserSuid, &ptransaction.UserTelephone{
			TelephoneId:         telephoneID,
			TelephoneStatusType: penum.TelephoneStatusType_TelephoneStatusType_Unlocked,
		}); err != nil {
			t.Fatal(err)
		}
	}
	character, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, rewardCharacterID)
	if err != nil {
		t.Fatal(err)
	}
	reliabilityBefore := character.Reliability

	client := papi.NewMessageClient(testConfig.GrpcConn)
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Receive(grpcCtx, &papi.MessageReceiveRequest{MessageIds: []string{master.Id}})
	cancel()
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("early Message.Receive returned %v, want FailedPrecondition", err)
	}
	dueTime := time.Now().UnixMilli()
	if err := opuser.UpsertUserMessageSchedule(ctx, testConfig.TestUserSuid, &ptransaction.UserMessageSchedule{
		MessageId: master.Id, ScheduledTime: dueTime,
	}); err != nil {
		t.Fatal(err)
	}
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	receive, err := client.Receive(grpcCtx, &papi.MessageReceiveRequest{MessageIds: []string{master.Id}})
	cancel()
	if err != nil {
		t.Fatalf("Message.Receive failed: %v", err)
	}
	if len(receive.GetCommonResponse().GetUpdatedData().GetMessages()) != 1 ||
		receive.GetCommonResponse().GetUpdatedData().GetMessages()[0].GetArrivedTime() != dueTime ||
		len(receive.GetCommonResponse().GetUpdatedData().GetMessageSchedules()) != 1 {
		t.Fatalf("Message.Receive returned the wrong post-image: %v", receive)
	}
	message, err = opuser.FindUserMessage(ctx, testConfig.TestUserSuid, master.Id)
	if err != nil || message.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Received ||
		message.GetArrivedTime() != dueTime {
		t.Fatalf("Message.Receive was not persisted: message=%v err=%v", message, err)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	groups, err := client.ListGroup(grpcCtx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("Message.ListGroup failed: %v", err)
	}
	if !hasMessageGroup(groups.Infos, master.MessageGroupId) {
		t.Fatalf("Message.ListGroup omitted %q", master.MessageGroupId)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	timeline, err := client.Timeline(grpcCtx, &papi.MessageTimelineRequest{MessageGroupId: master.MessageGroupId})
	cancel()
	if err != nil {
		t.Fatalf("Message.Timeline failed: %v", err)
	}
	info := findTimelineInfo(t, timeline.Infos, master.Id)

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Save(grpcCtx, &papi.MessageSaveRequest{
		MessageGroupId: master.MessageGroupId, TimelineId: info.TimelineId,
		MessageDetailId: firstDetail.MessageDetailId,
	})
	cancel()
	if err != nil {
		t.Fatalf("Message.Save failed: %v", err)
	}
	progress, err := opuser.FindUserMessageProgress(ctx, testConfig.TestUserSuid, master.Id)
	if err != nil {
		t.Fatal(err)
	}
	if progress.LastMessageDetailId != firstDetail.MessageDetailId || progress.LastMessageDetailTime <= 0 {
		t.Fatalf("Message.Save did not persist progress: %v", progress)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SaveHistory(grpcCtx, &papi.MessageSaveHistoryRequest{
		MessageId: master.Id, SelectMessageDetailIds: selections,
	})
	cancel()
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unfinished Message.SaveHistory returned %v, want FailedPrecondition", err)
	}
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Save(grpcCtx, &papi.MessageSaveRequest{
		MessageGroupId:         master.MessageGroupId,
		TimelineId:             info.TimelineId,
		MessageDetailId:        terminal.GetMessageDetailId(),
		SelectMessageDetailIds: selections,
	})
	cancel()
	if err != nil {
		t.Fatalf("Message.Save did not advance along the existing path: %v", err)
	}
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Finish(grpcCtx, &papi.MessageFinishRequest{
		MessageGroupId:         master.MessageGroupId,
		TimelineId:             info.TimelineId,
		SelectMessageDetailIds: alternateSelections,
	})
	cancel()
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("Message.Finish switched away from saved progress: err=%v", err)
	}
	message, err = opuser.FindUserMessage(ctx, testConfig.TestUserSuid, master.Id)
	if err != nil || message.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Received {
		t.Fatalf("conflicting Message.Finish changed message state: message=%v err=%v", message, err)
	}
	character, err = opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, rewardCharacterID)
	if err != nil || character.GetReliability() != reliabilityBefore {
		t.Fatalf("conflicting Message.Finish granted reliability: character=%v err=%v", character, err)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	finish, err := client.Finish(grpcCtx, &papi.MessageFinishRequest{
		MessageGroupId: master.MessageGroupId, TimelineId: info.TimelineId,
		SelectMessageDetailIds: selections,
	})
	cancel()
	if err != nil {
		t.Fatalf("Message.Finish failed: %v", err)
	}
	if !finish.IsReliabilityUp {
		t.Fatal("Message.Finish did not report the positive reliability reward")
	}
	message, err = opuser.FindUserMessage(ctx, testConfig.TestUserSuid, master.Id)
	if err != nil {
		t.Fatal(err)
	}
	if message.MessageStatusType != penum.MessageStatusType_MessageStatusType_Finished ||
		!slices.Equal(message.GetSelectMessageDetailIds(), selections) {
		t.Fatalf("Message.Finish was not persisted: %v", message)
	}
	character, err = opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, rewardCharacterID)
	if err != nil {
		t.Fatal(err)
	}
	if character.Reliability != reliabilityBefore+terminal.Reliability {
		t.Fatalf("Message.Finish reliability = %d, want %d", character.Reliability, reliabilityBefore+terminal.Reliability)
	}
	updatedCharacter := findUserCharacterByID(finish.GetCommonResponse().GetUpdatedData().GetCharacters(), rewardCharacterID)
	if updatedCharacter == nil || !proto.Equal(updatedCharacter, character) {
		t.Fatalf(
			"Message.Finish character post-image for terminal character %q = %v, want stored %v",
			rewardCharacterID, updatedCharacter, character,
		)
	}
	progress, err = opuser.FindUserMessageProgress(ctx, testConfig.TestUserSuid, master.Id)
	if err != nil || progress.LastMessageDetailId != terminal.MessageDetailId {
		t.Fatalf("Message.Finish terminal progress = %v, want %q (err=%v)", progress, terminal.MessageDetailId, err)
	}
	if _, err := opuser.FindUserMessageSchedule(ctx, testConfig.TestUserSuid, master.Id); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("Message.Finish did not remove its schedule: %v", err)
	}
	for _, telephoneID := range selectedMessageTelephoneIDs(master, selections) {
		telephone, err := opuser.FindUserTelephone(ctx, testConfig.TestUserSuid, telephoneID)
		if err != nil || telephone.GetTelephoneStatusType() != penum.TelephoneStatusType_TelephoneStatusType_Finished ||
			telephone.GetUnlockedTime() <= 0 || telephone.GetScheduledTime() != telephone.GetUnlockedTime() {
			t.Fatalf("Message.Finish did not persist embedded telephone %q: telephone=%v err=%v", telephoneID, telephone, err)
		}
		updatedTelephone := findUserTelephoneByID(finish.GetCommonResponse().GetUpdatedData().GetTelephones(), telephoneID)
		if updatedTelephone == nil || !proto.Equal(updatedTelephone, telephone) {
			t.Fatalf("Message.Finish embedded telephone post-image %q = %v, want %v", telephoneID, updatedTelephone, telephone)
		}
	}
	finishedMessage := proto.Clone(message).(*ptransaction.UserMessage)
	finishedCharacter := proto.Clone(character).(*ptransaction.UserCharacter)
	finishedProgress := *progress

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Save(grpcCtx, &papi.MessageSaveRequest{
		MessageGroupId: master.MessageGroupId, TimelineId: info.TimelineId,
		MessageDetailId: firstDetail.MessageDetailId,
	})
	cancel()
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("finished Message.Save returned %v, want FailedPrecondition", err)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := client.Finish(grpcCtx, &papi.MessageFinishRequest{
		MessageGroupId: master.MessageGroupId, TimelineId: info.TimelineId,
		SelectMessageDetailIds: alternateSelections,
	})
	cancel()
	if err != nil {
		t.Fatalf("repeated Message.Finish failed: %v", err)
	}
	if repeat.IsReliabilityUp {
		t.Fatal("repeated Message.Finish granted reliability twice")
	}
	message, err = opuser.FindUserMessage(ctx, testConfig.TestUserSuid, master.Id)
	if err != nil || !proto.Equal(message, finishedMessage) {
		t.Fatalf("finished message changed after Save/Finish replay: before=%v after=%v err=%v", finishedMessage, message, err)
	}
	character, err = opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, rewardCharacterID)
	if err != nil || !proto.Equal(character, finishedCharacter) {
		t.Fatalf("message reliability changed after Save/Finish replay: before=%v after=%v err=%v", finishedCharacter, character, err)
	}
	progress, err = opuser.FindUserMessageProgress(ctx, testConfig.TestUserSuid, master.Id)
	if err != nil || progress.LastMessageDetailId != finishedProgress.LastMessageDetailId ||
		progress.LastMessageDetailTime != finishedProgress.LastMessageDetailTime {
		t.Fatalf("message progress changed after Save/Finish replay: before=%v after=%v err=%v", finishedProgress, progress, err)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SaveHistory(grpcCtx, &papi.MessageSaveHistoryRequest{
		MessageId: master.Id, SelectMessageDetailIds: selections,
	})
	cancel()
	if err != nil {
		t.Fatalf("Message.SaveHistory failed: %v", err)
	}
	message, err = opuser.FindUserMessage(ctx, testConfig.TestUserSuid, master.Id)
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range selections {
		if !slices.Contains(message.SelectMessageDetailIds, selection) {
			t.Fatalf("Message.SaveHistory omitted selection %q: %v", selection, message.SelectMessageDetailIds)
		}
	}
}

func TestTelephoneScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	testTelephoneHistory(t)
}

func TestMessageDependencySchedulingScenario(t *testing.T) {
	setupTestUser(t)
	callUserGod(t, &admin.UserGodRequest{
		ServerUserId: testConfig.TestUserSuid,
		Feature:      admin.GodFeature{Message: umodel.GodMessageFeatureNone},
	})
	ctx := context.Background()
	if err := opuser.DeleteUserGodState(ctx, testConfig.TestUserSuid); err != nil {
		t.Fatal(err)
	}
	source, dependent, storyID := findMessageDependencyChain(t)

	storyClient := papi.NewStoryClient(testConfig.GrpcConn)
	beforeRead := time.Now().UnixMilli()
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	read, err := storyClient.Read(grpcCtx, &papi.StoryReadRequest{StoryId: storyID})
	cancel()
	if err != nil {
		t.Fatalf("Story.Read did not unlock source message: %v", err)
	}
	afterRead := time.Now().UnixMilli()
	if findUserMessageByID(read.GetCommonResponse().GetUpdatedData().GetMessages(), source.GetId()) == nil {
		t.Fatalf("Story.Read response omitted unlocked message %q: %v", source.GetId(), read)
	}
	message, err := opuser.FindUserMessage(ctx, testConfig.TestUserSuid, source.GetId())
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := opuser.FindUserMessageSchedule(ctx, testConfig.TestUserSuid, source.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if schedule.GetScheduledTime() < beforeRead || schedule.GetScheduledTime() > afterRead+time.Hour.Milliseconds() {
		t.Fatalf("story message delay is outside 0..1h: message=%v schedule=%v", message, schedule)
	}
	messageClient := papi.NewMessageClient(testConfig.GrpcConn)
	if message.GetMessageStatusType() == penum.MessageStatusType_MessageStatusType_Scheduled {
		schedule.ScheduledTime = time.Now().UnixMilli()
		if err := opuser.UpsertUserMessageSchedule(ctx, testConfig.TestUserSuid, schedule); err != nil {
			t.Fatal(err)
		}
		grpcCtx, cancel, err = getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		_, err = messageClient.Receive(grpcCtx, &papi.MessageReceiveRequest{MessageIds: []string{source.GetId()}})
		cancel()
		if err != nil {
			t.Fatalf("Message.Receive for story message failed: %v", err)
		}
	} else if message.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Received {
		t.Fatalf("Story.Read source message is not active: %v", message)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	timeline, err := messageClient.Timeline(grpcCtx, &papi.MessageTimelineRequest{MessageGroupId: source.GetMessageGroupId()})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	info := findTimelineInfo(t, timeline.GetInfos(), source.GetId())
	selections, _, err := messagepath.FirstPath(source)
	if err != nil {
		t.Fatal(err)
	}
	beforeFinish := time.Now().UnixMilli()
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	finish, err := messageClient.Finish(grpcCtx, &papi.MessageFinishRequest{
		MessageGroupId:         source.GetMessageGroupId(),
		TimelineId:             info.GetTimelineId(),
		SelectMessageDetailIds: selections,
	})
	cancel()
	if err != nil {
		t.Fatalf("Message.Finish did not advance dependency queue: %v", err)
	}
	afterFinish := time.Now().UnixMilli()
	if findUserMessageByID(finish.GetCommonResponse().GetUpdatedData().GetMessages(), dependent.GetId()) == nil {
		t.Fatalf("Message.Finish response omitted dependent message %q: %v", dependent.GetId(), finish)
	}
	dependentMessage, err := opuser.FindUserMessage(ctx, testConfig.TestUserSuid, dependent.GetId())
	if err != nil {
		t.Fatal(err)
	}
	dependentSchedule, err := opuser.FindUserMessageSchedule(ctx, testConfig.TestUserSuid, dependent.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if dependentSchedule.GetScheduledTime() < beforeFinish ||
		dependentSchedule.GetScheduledTime() > afterFinish+12*time.Hour.Milliseconds() {
		t.Fatalf("dependent message delay is outside 0..12h: message=%v schedule=%v", dependentMessage, dependentSchedule)
	}
	switch dependentMessage.GetMessageStatusType() {
	case penum.MessageStatusType_MessageStatusType_Scheduled:
		if dependentMessage.GetArrivedTime() != 0 {
			t.Fatalf("future dependent message already has an arrival: %v", dependentMessage)
		}
	case penum.MessageStatusType_MessageStatusType_Received:
		if dependentMessage.GetArrivedTime() != dependentSchedule.GetScheduledTime() {
			t.Fatalf("immediate dependent message arrival differs from schedule: message=%v schedule=%v", dependentMessage, dependentSchedule)
		}
	default:
		t.Fatalf("dependent message has invalid status: %v", dependentMessage)
	}
}

func findWearableCostumePair(t *testing.T) (*pmaster.Costume, *pmaster.Hair) {
	t.Helper()
	ctx := context.Background()
	costumes, err := opmaster.FindAllCostumes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	hairs, err := opmaster.FindAllHairs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, costume := range costumes {
		if costume.IsUserUnavailable || costume.IsDisableAdmin {
			continue
		}
		if _, err := opuser.FindUserCostume(ctx, testConfig.TestUserSuid, costume.Id); err != nil {
			continue
		}
		for _, hair := range hairs {
			if hair.CharacterId != costume.CharacterId || hair.IsDisableAdmin || slices.Contains(hair.NotWearableCostumeIds, costume.Id) ||
				len(hair.WearableCostumeIds) > 0 && !slices.Contains(hair.WearableCostumeIds, costume.Id) {
				continue
			}
			if _, err := opuser.FindUserHair(ctx, testConfig.TestUserSuid, hair.Id); err == nil {
				return costume, hair
			}
		}
	}
	t.Fatal("no owned compatible costume and hair pair exists")
	return nil, nil
}

func assertWardrobeChecked(t *testing.T, costumeId, hairId string, costumeChecked, hairChecked bool) {
	t.Helper()
	ctx := context.Background()
	costume, err := opuser.FindUserCostume(ctx, testConfig.TestUserSuid, costumeId)
	if err != nil {
		t.Fatal(err)
	}
	hair, err := opuser.FindUserHair(ctx, testConfig.TestUserSuid, hairId)
	if err != nil {
		t.Fatal(err)
	}
	if costume.Checked != costumeChecked || hair.Checked != hairChecked {
		t.Fatalf(
			"wardrobe checked state = (%v, %v), want (%v, %v)",
			costume.Checked, hair.Checked, costumeChecked, hairChecked,
		)
	}
}

func findRewardedUnreadStory(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	stories, err := opmaster.FindAllStories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, story := range stories {
		if story.RewardId == "" {
			continue
		}
		if parts := strings.Split(story.RewardId, ";"); len(parts) == 4 && parts[0] == "rd_auto" && parts[1] == "stone" {
			amount, parseErr := strconv.ParseInt(parts[3], 10, 32)
			userStory, findErr := opuser.FindUserStory(ctx, testConfig.TestUserSuid, story.Id)
			if parseErr == nil && amount > 0 && findErr == nil &&
				userStory.StatusType == penum.StoryStatusType_StoryStatusType_Unread && !userStory.IsInvalid {
				return story.Id
			}
			continue
		}
		reward, err := opmaster.FindReward(ctx, story.RewardId)
		if err != nil || len(reward.Resources) == 0 {
			continue
		}
		onlyStone := true
		for _, resource := range reward.Resources {
			if resource.ResourceType != penum.ResourceType_ResourceType_Stone || resource.Amount <= 0 {
				onlyStone = false
				break
			}
		}
		if !onlyStone {
			continue
		}
		userStory, err := opuser.FindUserStory(ctx, testConfig.TestUserSuid, story.Id)
		if err == nil && userStory.StatusType == penum.StoryStatusType_StoryStatusType_Unread && !userStory.IsInvalid {
			return story.Id
		}
	}
	t.Fatal("no unread stone-reward story exists")
	return ""
}

func assertStoryRead(t *testing.T, storyId string) {
	t.Helper()
	story, err := opuser.FindUserStory(context.Background(), testConfig.TestUserSuid, storyId)
	if err != nil {
		t.Fatal(err)
	}
	if story.StatusType != penum.StoryStatusType_StoryStatusType_Read {
		t.Fatalf("story %q status = %s, want Read", storyId, story.StatusType)
	}
}

func findUnlockedEventEpisode(t *testing.T, infos []*papi.EventStoryInfo, excluded string) (string, int32, string) {
	t.Helper()
	ctx := context.Background()
	for _, info := range infos {
		master, err := opmaster.FindEventStory(ctx, info.Id)
		if err != nil {
			continue
		}
		for _, episode := range info.Episodes {
			if !episode.Unlocked {
				continue
			}
			for _, masterEpisode := range master.Episodes {
				if masterEpisode.Episode == episode.Episode && masterEpisode.StoryId != "" && masterEpisode.StoryId != excluded {
					return info.Id, episode.Episode, masterEpisode.StoryId
				}
			}
		}
	}
	t.Fatal("no unlocked event story episode exists")
	return "", 0, ""
}

func findUnlockedExtraEpisode(t *testing.T, parts []*papi.ExtraStoryPartInfo, excluded ...string) (string, int32, string) {
	t.Helper()
	ctx := context.Background()
	for _, part := range parts {
		if !part.Unlocked {
			continue
		}
		for _, story := range part.Stories {
			if !story.Unlocked {
				continue
			}
			master, err := opmaster.FindExtraStory(ctx, story.Id)
			if err != nil {
				continue
			}
			for _, episode := range story.Episodes {
				if !episode.Unlocked {
					continue
				}
				for _, masterEpisode := range master.Episodes {
					if masterEpisode.Episode == episode.Episode && masterEpisode.StoryId != "" && !slices.Contains(excluded, masterEpisode.StoryId) {
						return story.Id, episode.Episode, masterEpisode.StoryId
					}
				}
			}
		}
	}
	t.Fatal("no unlocked extra story episode exists")
	return "", 0, ""
}

func sortExtraStoryPartInfos(infos []*papi.ExtraStoryPartInfo) {
	sort.Slice(infos, func(i, j int) bool { return infos[i].GetId() < infos[j].GetId() })
	for _, info := range infos {
		sort.Slice(info.Stories, func(i, j int) bool { return info.Stories[i].GetId() < info.Stories[j].GetId() })
		for _, story := range info.Stories {
			sort.Slice(story.Episodes, func(i, j int) bool {
				return story.Episodes[i].GetEpisode() < story.Episodes[j].GetEpisode()
			})
		}
	}
}

func findCompanyEnjoyStory(t *testing.T, excluded ...string) (string, int32, string) {
	t.Helper()
	pattern := regexp.MustCompile(`^st-company-bond-(.+)-01-([0-9]+)$`)
	stories, err := opmaster.FindAllStories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, story := range stories {
		matches := pattern.FindStringSubmatch(story.Id)
		if len(matches) != 3 || slices.Contains(excluded, story.Id) {
			continue
		}
		number, err := strconv.ParseInt(matches[2], 10, 32)
		if err != nil {
			continue
		}
		userStory, err := opuser.FindUserStory(context.Background(), testConfig.TestUserSuid, story.Id)
		if err == nil && userStory.StatusType == penum.StoryStatusType_StoryStatusType_Unread {
			return "char-" + matches[1], int32(number), story.Id
		}
	}
	t.Fatal("no unread company-enjoy story exists")
	return "", 0, ""
}

func testDiaryReadFlow(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if err := opuser.UpsertUserDiaryState(ctx, &umodel.UserDiaryState{SUID: testConfig.TestUserSuid, ReadIds: []string{}}); err != nil {
		t.Fatal(err)
	}
	client := papi.NewDiaryClient(testConfig.GrpcConn)
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	infos, err := client.GetInfos(grpcCtx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("Diary.GetInfos failed: %v", err)
	}
	if len(infos.Infos) == 0 || infos.Infos[0].IsRead {
		t.Fatalf("Diary.GetInfos returned an invalid unread catalog: %v", infos.Infos)
	}
	diaryId := infos.Infos[0].Id
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Read(grpcCtx, &papi.DiaryReadRequest{DiaryIds: []string{diaryId}})
	cancel()
	if err != nil {
		t.Fatalf("Diary.Read failed: %v", err)
	}
	state, err := opuser.FindUserDiaryState(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(state.ReadIds, diaryId) {
		t.Fatalf("Diary.Read did not persist %q: %v", diaryId, state)
	}
}

func findReliabilityMessage(t *testing.T) (*pmaster.Message, *pmaster.MessageDetail, []string, *pmaster.MessageDetail) {
	t.Helper()
	masters, err := opmaster.FindAllMessages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var fallbackMaster *pmaster.Message
	var fallbackTerminal, fallbackFirst *pmaster.MessageDetail
	var fallbackSelections []string
	for _, master := range masters {
		if master.MessageGroupId == "" || len(master.Details) == 0 {
			continue
		}
		terminal, selections, first := findPositiveReliabilityPath(master)
		if terminal == nil {
			continue
		}
		characterID := terminal.GetCharacterId()
		if characterID == "" {
			characterID = master.GetCharacterId()
		}
		if characterID == "" {
			continue
		}
		if _, err := opuser.FindUserCharacter(context.Background(), testConfig.TestUserSuid, characterID); err != nil {
			continue
		}
		if _, ok := findAlternateMessagePath(master, terminal.GetMessageDetailId(), selections); !ok {
			continue
		}
		if len(selectedMessageTelephoneIDs(master, selections)) > 0 {
			return master, terminal, selections, first
		}
		if terminal.GetCharacterId() != "" && terminal.GetCharacterId() != master.GetCharacterId() {
			return master, terminal, selections, first
		}
		if fallbackMaster == nil {
			fallbackMaster, fallbackTerminal = master, terminal
			fallbackSelections, fallbackFirst = selections, first
		}
	}
	if fallbackMaster != nil {
		return fallbackMaster, fallbackTerminal, fallbackSelections, fallbackFirst
	}
	t.Fatal("no message with a positive-reliability terminal path exists")
	return nil, nil, nil, nil
}

func findMessageDependencyChain(t *testing.T) (*pmaster.Message, *pmaster.Message, string) {
	t.Helper()
	ctx := context.Background()
	masters, err := opmaster.FindAllMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	masterByID := make(map[string]*pmaster.Message, len(masters))
	for _, master := range masters {
		masterByID[master.GetId()] = master
	}
	activeGroups := make(map[string]struct{})
	userMessages, err := opuser.FindAllUserMessages(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range userMessages {
		if message.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Scheduled &&
			message.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Received {
			continue
		}
		if master := masterByID[message.GetMessageId()]; master != nil {
			activeGroups[master.GetMessageGroupId()] = struct{}{}
		}
	}
	conditions, err := opmaster.FindAllConditions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conditionByID := make(map[string]*pmaster.Condition, len(conditions))
	for _, condition := range conditions {
		conditionByID[condition.GetId()] = condition
	}
	for _, dependent := range masters {
		condition := conditionByID[dependent.GetUnlockConditionId()]
		if condition == nil {
			continue
		}
		sourceID := ""
		for _, setting := range condition.GetSettings() {
			if setting.GetFinishedMessage().GetMessageId() != "" {
				sourceID = setting.GetFinishedMessage().GetMessageId()
				break
			}
		}
		source := masterByID[sourceID]
		if source == nil || source.GetMessageGroupId() != dependent.GetMessageGroupId() {
			continue
		}
		if _, active := activeGroups[source.GetMessageGroupId()]; active {
			continue
		}
		sourceCondition := conditionByID[source.GetUnlockConditionId()]
		if sourceCondition == nil {
			continue
		}
		storyID := ""
		for _, setting := range sourceCondition.GetSettings() {
			if setting.GetReadStory().GetStoryId() != "" {
				storyID = setting.GetReadStory().GetStoryId()
				break
			}
		}
		if storyID == "" {
			continue
		}
		story, err := opuser.FindUserStory(ctx, testConfig.TestUserSuid, storyID)
		if err != nil || story.GetStatusType() != penum.StoryStatusType_StoryStatusType_Unread || story.GetIsInvalid() {
			continue
		}
		if _, err := opuser.FindUserMessage(ctx, testConfig.TestUserSuid, source.GetId()); !errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if _, err := opuser.FindUserMessage(ctx, testConfig.TestUserSuid, dependent.GetId()); !errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		return source, dependent, storyID
	}
	t.Fatal("no unread-story to finished-message dependency chain exists")
	return nil, nil, ""
}

func findPositiveReliabilityPath(master *pmaster.Message) (*pmaster.MessageDetail, []string, *pmaster.MessageDetail) {
	details := make(map[string]*pmaster.MessageDetail, len(master.Details))
	indegree := make(map[string]int, len(master.Details))
	for _, detail := range master.Details {
		if detail.MessageDetailId == "" || details[detail.MessageDetailId] != nil {
			return nil, nil, nil
		}
		details[detail.MessageDetailId] = detail
		indegree[detail.MessageDetailId] = 0
	}
	for _, detail := range master.Details {
		for _, nextId := range detail.NextMessageDetailIds {
			if details[nextId] == nil {
				return nil, nil, nil
			}
			indegree[nextId]++
		}
	}
	var root *pmaster.MessageDetail
	for id, degree := range indegree {
		if degree == 0 {
			if root != nil {
				return nil, nil, nil
			}
			root = details[id]
		}
	}
	if root == nil {
		return nil, nil, nil
	}
	type pathState struct {
		detail     *pmaster.MessageDetail
		selections []string
		visited    map[string]struct{}
	}
	stack := []pathState{{detail: root, visited: map[string]struct{}{}}}
	for len(stack) > 0 {
		state := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, seen := state.visited[state.detail.MessageDetailId]; seen {
			continue
		}
		visited := make(map[string]struct{}, len(state.visited)+1)
		for id := range state.visited {
			visited[id] = struct{}{}
		}
		visited[state.detail.MessageDetailId] = struct{}{}
		if len(state.detail.NextMessageDetailIds) == 0 {
			if state.detail.Reliability > 0 {
				return state.detail, state.selections, root
			}
			continue
		}
		if len(state.detail.NextMessageDetailIds) > 1 {
			validChoices := true
			for _, nextID := range state.detail.NextMessageDetailIds {
				if details[nextID].GetChoiceText() == "" {
					validChoices = false
					break
				}
			}
			if !validChoices {
				continue
			}
		}
		for index := len(state.detail.NextMessageDetailIds) - 1; index >= 0; index-- {
			next := details[state.detail.NextMessageDetailIds[index]]
			selections := append([]string(nil), state.selections...)
			if next.GetChoiceText() != "" {
				selections = append(selections, next.MessageDetailId)
			}
			stack = append(stack, pathState{detail: next, selections: selections, visited: visited})
		}
	}
	return nil, nil, root
}

func findAlternateMessagePath(master *pmaster.Message, usedTerminalID string, usedSelections []string) ([]string, bool) {
	details := make(map[string]*pmaster.MessageDetail, len(master.GetDetails()))
	indegree := make(map[string]int, len(master.GetDetails()))
	for _, detail := range master.GetDetails() {
		if detail.GetMessageDetailId() == "" || details[detail.GetMessageDetailId()] != nil {
			return nil, false
		}
		details[detail.GetMessageDetailId()] = detail
		indegree[detail.GetMessageDetailId()] = 0
	}
	for _, detail := range master.GetDetails() {
		for _, nextID := range detail.GetNextMessageDetailIds() {
			if details[nextID] == nil {
				return nil, false
			}
			indegree[nextID]++
		}
	}
	var root *pmaster.MessageDetail
	for id, degree := range indegree {
		if degree != 0 {
			continue
		}
		if root != nil {
			return nil, false
		}
		root = details[id]
	}
	if root == nil {
		return nil, false
	}

	type pathState struct {
		detail     *pmaster.MessageDetail
		selections []string
		visited    map[string]struct{}
	}
	stack := []pathState{{detail: root, visited: map[string]struct{}{}}}
	var sameTerminalAlternative []string
	for len(stack) > 0 {
		state := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, duplicate := state.visited[state.detail.GetMessageDetailId()]; duplicate {
			continue
		}
		visited := make(map[string]struct{}, len(state.visited)+1)
		for id := range state.visited {
			visited[id] = struct{}{}
		}
		visited[state.detail.GetMessageDetailId()] = struct{}{}
		nextIDs := state.detail.GetNextMessageDetailIds()
		if len(nextIDs) == 0 {
			if slices.Equal(state.selections, usedSelections) {
				continue
			}
			if state.detail.GetMessageDetailId() != usedTerminalID {
				return state.selections, true
			}
			if sameTerminalAlternative == nil {
				sameTerminalAlternative = append([]string(nil), state.selections...)
			}
			continue
		}
		if len(nextIDs) > 1 {
			validChoices := true
			for _, nextID := range nextIDs {
				if details[nextID].GetChoiceText() == "" {
					validChoices = false
					break
				}
			}
			if !validChoices {
				continue
			}
		}
		for index := len(nextIDs) - 1; index >= 0; index-- {
			next := details[nextIDs[index]]
			selections := append([]string(nil), state.selections...)
			if next.GetChoiceText() != "" {
				selections = append(selections, next.GetMessageDetailId())
			}
			stack = append(stack, pathState{detail: next, selections: selections, visited: visited})
		}
	}
	return sameTerminalAlternative, sameTerminalAlternative != nil
}

func findUserCharacterByID(characters []*ptransaction.UserCharacter, characterID string) *ptransaction.UserCharacter {
	for _, character := range characters {
		if character.GetCharacterId() == characterID {
			return character
		}
	}
	return nil
}

func findUserMessageByID(messages []*ptransaction.UserMessage, messageID string) *ptransaction.UserMessage {
	for _, message := range messages {
		if message.GetMessageId() == messageID {
			return message
		}
	}
	return nil
}

func findUserTelephoneByID(telephones []*ptransaction.UserTelephone, telephoneID string) *ptransaction.UserTelephone {
	for _, telephone := range telephones {
		if telephone.GetTelephoneId() == telephoneID {
			return telephone
		}
	}
	return nil
}

func selectedMessageTelephoneIDs(master *pmaster.Message, selections []string) []string {
	details := make(map[string]*pmaster.MessageDetail, len(master.GetDetails()))
	indegree := make(map[string]int, len(master.GetDetails()))
	for _, detail := range master.GetDetails() {
		details[detail.GetMessageDetailId()] = detail
		indegree[detail.GetMessageDetailId()] = 0
	}
	for _, detail := range master.GetDetails() {
		for _, nextID := range detail.GetNextMessageDetailIds() {
			indegree[nextID]++
		}
	}
	var current *pmaster.MessageDetail
	for id, degree := range indegree {
		if degree == 0 {
			current = details[id]
			break
		}
	}
	selected := make(map[string]struct{}, len(selections))
	for _, selection := range selections {
		selected[selection] = struct{}{}
	}
	result := make([]string, 0)
	visited := make(map[string]struct{}, len(details))
	for current != nil {
		if _, duplicate := visited[current.GetMessageDetailId()]; duplicate {
			break
		}
		visited[current.GetMessageDetailId()] = struct{}{}
		if current.GetTelephoneId() != "" {
			result = append(result, current.GetTelephoneId())
		}
		nextIDs := current.GetNextMessageDetailIds()
		if len(nextIDs) == 0 {
			break
		}
		nextID := nextIDs[0]
		if len(nextIDs) > 1 {
			for _, candidateID := range nextIDs {
				if _, ok := selected[candidateID]; ok {
					nextID = candidateID
					break
				}
			}
		}
		current = details[nextID]
	}
	return result
}

func hasMessageGroup(infos []*papi.MessageGroupInfo, id string) bool {
	for _, info := range infos {
		if info.MessageGroupId == id {
			return true
		}
	}
	return false
}

func findTimelineInfo(t *testing.T, infos []*papi.TimelineInfo, messageId string) *papi.TimelineInfo {
	t.Helper()
	for _, info := range infos {
		if info.MessageId == messageId {
			return info
		}
	}
	t.Fatalf("Message.Timeline omitted %q", messageId)
	return nil
}

func testTelephoneHistory(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	telephones, err := opuser.FindAllUserTelephones(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, telephone := range telephones {
		if _, err := opmaster.FindTelephone(ctx, telephone.TelephoneId); err != nil {
			continue
		}
		telephone.TelephoneStatusType = penum.TelephoneStatusType_TelephoneStatusType_Scheduled
		telephone.UnlockedTime = time.Now().Add(-time.Minute).UnixMilli()
		telephone.ScheduledTime = time.Now().Add(time.Minute).UnixMilli()
		if err := opuser.UpsertUserTelephone(ctx, testConfig.TestUserSuid, telephone); err != nil {
			t.Fatal(err)
		}
		client := papi.NewTelephoneClient(testConfig.GrpcConn)
		grpcCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Receive(grpcCtx, &papi.TelephoneReceiveRequest{TelephoneIds: []string{telephone.TelephoneId}})
		cancel()
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("early Telephone.Receive returned %v, want FailedPrecondition", err)
		}
		telephone.ScheduledTime = time.Now().Add(-time.Minute).UnixMilli()
		if err := opuser.UpsertUserTelephone(ctx, testConfig.TestUserSuid, telephone); err != nil {
			t.Fatal(err)
		}
		grpcCtx, cancel, err = getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		receive, err := client.Receive(grpcCtx, &papi.TelephoneReceiveRequest{TelephoneIds: []string{telephone.TelephoneId}})
		cancel()
		if err != nil {
			t.Fatalf("Telephone.Receive failed: %v", err)
		}
		if len(receive.GetCommonResponse().GetUpdatedData().GetTelephones()) != 1 ||
			receive.GetCommonResponse().GetUpdatedData().GetTelephones()[0].GetTelephoneStatusType() !=
				penum.TelephoneStatusType_TelephoneStatusType_Received {
			t.Fatalf("Telephone.Receive returned the wrong post-image: %v", receive)
		}
		grpcCtx, cancel, err = getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		finish, err := client.Finishes(grpcCtx, &papi.TelephoneFinishesRequest{TelephoneIds: []string{telephone.TelephoneId}})
		cancel()
		if err != nil {
			t.Fatalf("Telephone.Finishes failed: %v", err)
		}
		if finish.GetIsReliabilityUp() || len(finish.GetCommonResponse().GetUpdatedData().GetTelephones()) != 1 ||
			finish.GetCommonResponse().GetUpdatedData().GetTelephones()[0].GetTelephoneStatusType() !=
				penum.TelephoneStatusType_TelephoneStatusType_Finished {
			t.Fatalf("Telephone.Finishes returned the wrong post-image: %v", finish)
		}
		stored, err := opuser.FindUserTelephone(ctx, testConfig.TestUserSuid, telephone.TelephoneId)
		if err != nil || stored.GetTelephoneStatusType() != penum.TelephoneStatusType_TelephoneStatusType_Finished {
			t.Fatalf("Telephone.Finishes was not persisted: telephone=%v err=%v", stored, err)
		}
		grpcCtx, cancel, err = getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.CallHistory(grpcCtx, &papi.TelephoneCallHistoryRequest{TelephoneId: telephone.TelephoneId})
		cancel()
		if err != nil {
			t.Fatalf("Telephone.CallHistory failed: %v", err)
		}
		return
	}
	t.Fatal("no finished telephone exists")
}
