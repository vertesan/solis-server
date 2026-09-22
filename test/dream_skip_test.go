package test

import (
	"context"
	"errors"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestDreamSkipProgressScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewDreamClient(testConfig.GrpcConn)
	base, material := dreamTestPhotos(t, ctx, testConfig.TestUserSuid)
	initial := proto.Clone(base).(*ptransaction.UserPhoto)
	top := profileCall(t, client.Top, &papi.Empty{})
	manualID := top.GetSeasonInfo().GetGenerateAreaItemId()
	setCardTrainingItem(t, manualID, 1000)
	generated := profileCall(t, client.GenerateArea, &papi.DreamGenerateAreaRequest{BasePhotoId: base.GetPhotoId(), MaterialPhotoId: material.GetPhotoId()})
	afterGenerateManuals := questStartUsablePasses(t, manualID)
	if afterGenerateManuals >= 1000 || generated.GetDreamUserPhoto().GetGeneratedTime() <= 0 {
		t.Fatal("generation did not consume manuals/persist a generated photo")
	}
	listed := profileCall(t, client.ListQuest, &papi.DreamListQuestRequest{BasePhotoId: base.GetPhotoId()})
	limit := listed.GetMaxQuestNumber() * 4 / 5
	if limit <= 0 || len(listed.GetQuestInfos()) != int(listed.GetMaxQuestNumber()) {
		t.Fatal("invalid generated stage list")
	}
	deckNumber := questStartDeck(t, 1000000)
	request := &papi.DreamSkipQuestRequest{BasePhotoId: base.GetPhotoId(), DeckNumber: deckNumber, QuestNumber: limit}
	profileReject(t, codes.FailedPrecondition, client.SkipQuest, &papi.DreamSkipQuestRequest{BasePhotoId: base.GetPhotoId(), DeckNumber: deckNumber, QuestNumber: limit + 1})
	characters := map[string]bool{}
	for _, card := range questStartUserDeckCards(t, deckNumber) {
		master, err := opmaster.FindCard(ctx, card.GetCardId())
		if err != nil {
			t.Fatal(err)
		}
		characters[master.GetCharacterId()] = true
	}
	counts, beforePoints := map[string]int32{}, map[string]int32{}
	for _, q := range listed.GetQuestInfos() {
		if q.GetQuestNumber() <= limit {
			counts[q.GetMusicId()]++
		}
	}
	for musicID := range counts {
		rows, err := opuser.FindUserMusicMasteries(ctx, testConfig.TestUserSuid, musicID)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			beforePoints[musicID+":"+row.GetCharacterId()] = row.GetMasteryPoint()
		}
	}
	// Two simultaneous skips of the same range must grant progress only once.
	type answer struct {
		result *papi.DreamSkipQuestResponse
		err    error
	}
	answers := make(chan answer, 2)
	for range 2 {
		rpcCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			defer cancel()
			result, err := client.SkipQuest(rpcCtx, request)
			answers <- answer{result, err}
		}()
	}
	var response *papi.DreamSkipQuestResponse
	success, rejected := 0, 0
	for range 2 {
		a := <-answers
		if a.err == nil {
			success++
			response = a.result
		} else if status.Code(a.err) == codes.FailedPrecondition {
			rejected++
		} else {
			t.Fatal(a.err)
		}
	}
	if success != 1 || rejected != 1 || response.GetClearedQuestNumber() != limit {
		t.Fatalf("duplicate skip settled: success=%d rejected=%d", success, rejected)
	}
	if questStartUsablePasses(t, manualID) != afterGenerateManuals {
		t.Fatal("skipping consumed additional manuals")
	}
	for musicID, count := range counts {
		rows, err := opuser.FindUserMusicMasteries(ctx, testConfig.TestUserSuid, musicID)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, row := range rows {
			if characters[row.GetCharacterId()] {
				seen[row.GetCharacterId()] = true
				if row.GetMasteryPoint() != beforePoints[musicID+":"+row.GetCharacterId()]+count {
					t.Fatal("skip mastery was duplicated or failed to scale by stage count")
				}
			}
		}
		if len(seen) != len(characters) {
			t.Fatal("skip omitted a participating character's mastery")
		}
	}
	after, err := opuser.FindUserPhoto(ctx, testConfig.TestUserSuid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	if !after.GetIsDreamBase() || proto.Equal(initial.GetAbilities()[0], after.GetAbilities()[0]) && len(response.GetAbilityChangeInfos()) == 0 {
		t.Fatal("skip did not update photo growth")
	}
	profileReject(t, codes.FailedPrecondition, client.SkipQuest, request)
	progress := profileCall(t, client.ListQuest, &papi.DreamListQuestRequest{BasePhotoId: base.GetPhotoId()})
	for _, q := range progress.GetQuestInfos() {
		if q.GetCleared() != (q.GetQuestNumber() <= limit) {
			t.Fatal("quest clear flags disagree with skipped progress")
		}
	}
	profileCall(t, client.ResetArea, &papi.DreamResetAreaRequest{BasePhotoId: base.GetPhotoId()})
	reset, err := opuser.FindUserPhoto(ctx, testConfig.TestUserSuid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	if reset.GetIsDreamBase() || reset.GetDreamMaterialPhotoRarity() != 0 || len(reset.GetAbilities()) != len(initial.GetAbilities()) {
		t.Fatal("Dream reset did not restore the ordinary photo")
	}
	for i := range initial.GetAbilities() {
		if !proto.Equal(initial.GetAbilities()[i], reset.GetAbilities()[i]) {
			t.Fatal("Dream reset did not remove only the acquired option growth")
		}
	}
	if _, err := opuser.FindUserPhoto(ctx, testConfig.TestUserSuid, material.GetPhotoId()); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("reset refunded consumed material: %v", err)
	}
	if questStartUsablePasses(t, manualID) != afterGenerateManuals {
		t.Fatal("reset refunded consumed manuals")
	}
	if _, err := opuser.FindUserDreamArea(ctx, testConfig.TestUserSuid, base.GetPhotoId()); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("reset left generated stages: %v", err)
	}
}
