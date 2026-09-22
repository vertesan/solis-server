package test

import (
	"context"
	"math"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func TestDreamLiveSettlementScenario(t *testing.T) {
	setupTestUser(t)
	ctx, suid := context.Background(), testConfig.TestUserSuid
	client := papi.NewDreamClient(testConfig.GrpcConn)
	base, material := dreamTestPhotos(t, ctx, suid)
	top := profileCall(t, client.Top, &papi.Empty{})
	manualID := top.GetSeasonInfo().GetGenerateAreaItemId()
	setCardTrainingItem(t, manualID, 1000)
	profileCall(t, client.GenerateArea, &papi.DreamGenerateAreaRequest{BasePhotoId: base.GetPhotoId(), MaterialPhotoId: material.GetPhotoId()})
	deck := questStartDeck(t, 1000000)
	request := &papi.DreamStartQuestRequest{BasePhotoId: base.GetPhotoId(), DeckNumber: deck, IsSkip: true}
	profileReject(t, codes.InvalidArgument, client.StartQuest, &papi.DreamStartQuestRequest{BasePhotoId: base.GetPhotoId()})
	profileReject(t, codes.FailedPrecondition, client.StartQuest, &papi.DreamStartQuestRequest{BasePhotoId: base.GetPhotoId(), DeckNumber: deck, UserBuddyDeckPositions: []*ptransaction.UserDeckPosition{{Position: 1}}})
	area, err := opuser.FindUserDreamArea(ctx, suid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	if len(area.Quests) == 0 || area.Data.GetClearQuestNumber() != 0 {
		t.Fatal("new Dream area has no first stage")
	}
	// Alter only the configured test user's generated quest. This gives a
	// deterministic defeat without modifying shared master data or the engine.
	area.Quests[0].ClearScore = math.MaxInt64 / 4
	if err := opuser.UpsertUserDreamArea(ctx, area); err != nil {
		t.Fatal(err)
	}
	beforePhoto, err := opuser.FindUserPhoto(ctx, suid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	userBefore, err := opuser.FindUser(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	charactersBefore, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	cardsBefore := questStartUserDeckCards(t, deck)
	manualsBefore := questStartUsablePasses(t, manualID)
	// Dream prefetches the ordinary saved deck without a master quest ID:
	// the generated stage is selected by basePhotoId only when StartQuest runs.
	positionsBefore, err := opuser.FindUserDeckPositions(ctx, suid, deck)
	if err != nil {
		t.Fatal(err)
	}
	listed := profileCall(t, papi.NewQuestClient(testConfig.GrpcConn).ListAsset, &papi.QuestListAssetRequest{DeckNumber: deck})
	if len(listed.GetCharacterAssets()) != 5 || listed.GetCommonResponse() == nil {
		t.Fatal("Dream asset prefetch omitted the saved deck's five appearances or common response")
	}
	if proto.Size(listed.GetCommonResponse().GetUpdatedData()) != 0 || proto.Size(listed.GetCommonResponse().GetDeletedData()) != 0 {
		t.Fatal("Dream asset prefetch unexpectedly mutated client state")
	}
	afterPrefetch, err := opuser.FindUserDreamArea(ctx, suid, base.GetPhotoId())
	if err != nil || !proto.Equal(area.Data, afterPrefetch.Data) {
		t.Fatalf("Dream asset prefetch changed area progress: %v", err)
	}
	photoAfterPrefetch, err := opuser.FindUserPhoto(ctx, suid, base.GetPhotoId())
	if err != nil || !proto.Equal(beforePhoto, photoAfterPrefetch) || questStartUsablePasses(t, manualID) != manualsBefore {
		t.Fatalf("Dream asset prefetch changed the photo or generation manuals: %v", err)
	}
	positionsAfter, err := opuser.FindUserDeckPositions(ctx, suid, deck)
	if err != nil || len(positionsBefore) != len(positionsAfter) {
		t.Fatalf("Dream asset prefetch changed the saved deck: %v", err)
	}
	for i, position := range positionsBefore {
		if !proto.Equal(position, positionsAfter[i]) {
			t.Fatal("Dream asset prefetch persisted a cosmetic substitution")
		}
	}
	failure := profileCall(t, client.StartQuest, request)
	if failure.GetResult().GetCleared() || failure.GetClearedQuestNumber() != 0 || len(failure.GetAbilityChangeInfos()) != 0 || len(failure.GetCommonResponse().GetUpdatedData().GetPhotos()) != 0 || len(failure.GetMusicMasteryRewards()) != 0 {
		t.Fatal("failed Dream live advanced the stage or granted rewards")
	}
	if len(failure.GetResult().GetUserInfos()) == 0 || len(failure.GetResult().GetUserInfos()[0].GetUserDeck().GetCards()) != len(listed.GetCharacterAssets()) {
		t.Fatal("Dream live omitted the prefetched deck")
	}
	for i, card := range failure.GetResult().GetUserInfos()[0].GetUserDeck().GetCards() {
		asset := listed.GetCharacterAssets()[i]
		if card.GetDisplayCharacterId() != asset.GetCharacterId() || card.GetDisplayCostumeId() != asset.GetCostumeId() || card.GetDisplayHairId() != asset.GetHairId() {
			t.Fatalf("Dream slot %d differs from its prefetched appearance: %v", i+1, asset)
		}
	}
	area, err = opuser.FindUserDreamArea(ctx, suid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	if area.Data.GetLastChallengedTime() <= 0 || area.Data.GetClearQuestNumber() != 0 {
		t.Fatal("failed Dream live did not persist only its challenge time")
	}
	photo, err := opuser.FindUserPhoto(ctx, suid, base.GetPhotoId())
	if err != nil || !proto.Equal(beforePhoto, photo) {
		t.Fatalf("failed Dream live changed the photo: %v", err)
	}
	musicID := area.Quests[0].GetMusicId()
	mastery, err := opuser.FindUserMusicMasteries(ctx, suid, musicID)
	if err != nil || len(mastery) != 0 {
		t.Fatalf("failed Dream live granted mastery: %v", err)
	}
	// Exercise rollback after photo progress is applied, when mastery settlement
	// discovers an overflow. Neither progress nor option increases may persist.
	area.Quests[0].ClearScore = 1
	if err := opuser.UpsertUserDreamArea(ctx, area); err != nil {
		t.Fatal(err)
	}
	firstMaster, err := opmaster.FindCard(ctx, cardsBefore[0].GetCardId())
	if err != nil {
		t.Fatal(err)
	}
	overflow := &ptransaction.UserCharacterMusic{CharacterId: firstMaster.GetCharacterId(), MusicId: musicID, MasteryPoint: math.MaxInt32}
	if err := opuser.UpsertUserCharacterMusic(ctx, suid, overflow); err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.FailedPrecondition, client.StartQuest, request)
	afterRollback, err := opuser.FindUserDreamArea(ctx, suid, base.GetPhotoId())
	if err != nil || !proto.Equal(area.Data, afterRollback.Data) {
		t.Fatalf("failed mastery settlement retained Dream progress: %v", err)
	}
	photo, err = opuser.FindUserPhoto(ctx, suid, base.GetPhotoId())
	if err != nil || !proto.Equal(beforePhoto, photo) {
		t.Fatalf("failed mastery settlement retained photo growth: %v", err)
	}
	overflow.MasteryPoint = 0
	if err := opuser.UpsertUserCharacterMusic(ctx, suid, overflow); err != nil {
		t.Fatal(err)
	}
	success := profileCall(t, client.StartQuest, request)
	if !success.GetResult().GetCleared() || success.GetClearedQuestNumber() != 1 || success.GetDreamUserPhoto().GetClearQuestNumber() != 1 || len(success.GetCommonResponse().GetUpdatedData().GetPhotos()) != 1 || success.GetResult().GetDetailLogId() == "" || len(success.GetResult().GetCharts()) == 0 {
		t.Fatal("successful Dream live omitted trace or persisted photo progress")
	}
	if len(success.GetRankPatterns()) != 3 || success.GetRankPatterns()[1].GetThresholdScore() != 1 {
		t.Fatal("Dream live did not expose per-beat score display thresholds")
	}
	photo, err = opuser.FindUserPhoto(ctx, suid, base.GetPhotoId())
	if err != nil || !proto.Equal(photo, success.GetCommonResponse().GetUpdatedData().GetPhotos()[0]) {
		t.Fatalf("Dream photo response differs from persisted growth: %v", err)
	}
	for _, change := range success.GetAbilityChangeInfos() {
		if change.GetAfterEffectValue() < change.GetBeforeEffectValue() || change.GetAfterEffectValue() > change.GetMaxEffectValue() {
			t.Fatal("Dream option growth is outside its plan")
		}
	}
	mastery, err = opuser.FindUserMusicMasteries(ctx, suid, musicID)
	if err != nil {
		t.Fatal(err)
	}
	wantedCharacters := map[string]bool{}
	for _, card := range cardsBefore {
		master, err := opmaster.FindCard(ctx, card.GetCardId())
		if err != nil {
			t.Fatal(err)
		}
		wantedCharacters[master.GetCharacterId()] = true
	}
	if len(mastery) != len(wantedCharacters) {
		t.Fatal("Dream mastery did not use unique scoring characters")
	}
	for _, row := range mastery {
		if !wantedCharacters[row.GetCharacterId()] || row.GetMasteryPoint() != 1 {
			t.Fatal("Dream clear did not grant exactly one mastery point")
		}
	}
	userAfter, err := opuser.FindUser(ctx, suid)
	if err != nil || userAfter.GetManagerExp() != userBefore.GetManagerExp() {
		t.Fatalf("Dream clear granted manager EXP: %v", err)
	}
	for _, before := range cardsBefore {
		after, err := opuser.FindUserCard(ctx, suid, before.GetCardId())
		if err != nil || after.GetTotalExp() != before.GetTotalExp() {
			t.Fatalf("Dream clear granted card EXP: %v", err)
		}
	}
	for _, before := range charactersBefore {
		after, err := opuser.FindUserCharacter(ctx, suid, before.GetCharacterId())
		if err != nil || !proto.Equal(before, after) {
			t.Fatalf("Dream clear changed character fans or reliability: %v", err)
		}
	}
	if questStartUsablePasses(t, manualID) != manualsBefore {
		t.Fatal("Dream stage attempts consumed generation manuals")
	}
	area, err = opuser.FindUserDreamArea(ctx, suid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	area.Data.ClearQuestNumber = area.Data.GetMaxQuestNumber()
	if err := opuser.UpsertUserDreamArea(ctx, area); err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.FailedPrecondition, client.StartQuest, request)
}
