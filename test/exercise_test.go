package test

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func TestExerciseBasicLessonsScenario(t *testing.T) {
	setupExerciseTestUser(t)
	ctx, suid := context.Background(), testConfig.TestUserSuid
	client := papi.NewExerciseClient(testConfig.GrpcConn)
	// Keep a real saved unit present: rental lessons must not overwrite it or
	// borrow its inflated stats instead of the exercise master configuration.
	questStartDeck(t, 1000000)
	inventoryBefore := exerciseInventorySnapshot(t)
	userBefore, err := opuser.FindUser(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	top := profileCall(t, client.Top, &papi.Empty{})
	if !exerciseLesson(t, top, "exercise-basic-01").GetUnlocked() || exerciseLesson(t, top, "exercise-basic-02").GetUnlocked() {
		t.Fatal("fresh exercise progress did not unlock only the first lesson")
	}
	profileReject(t, codes.FailedPrecondition, client.Start, &papi.ExerciseStartRequest{ExerciseId: "exercise-basic-02"})

	// Captures 027/029: valid rental cards in the wrong attribute lanes fail.
	wrong := &papi.ExerciseSaveDeckRequest{ExerciseId: "exercise-basic-01", Positions: []*papi.ExerciseDeckPosition{
		{Position: 4, CardId: "card-chs-03-schl-00"},
		{Position: 5, CardId: "card-rei-03-schl-00"},
	}}
	profileCall(t, client.SaveDeck, wrong)
	failed := profileCall(t, client.Start, &papi.ExerciseStartRequest{ExerciseId: wrong.GetExerciseId(), IsSkip: true})
	var failedScore int64
	if users := failed.GetResult().GetUserInfos(); len(users) > 0 {
		failedScore = users[0].GetUserResult().GetScore()
	}
	if failed.GetResult() == nil || failed.GetResult().GetCleared() || len(failed.GetClearRewards()) != 0 {
		t.Fatalf("incorrect first-lesson formation passed or received a clear reward: cleared=%t score=%d", failed.GetResult().GetCleared(), failedScore)
	}
	quest, err := opuser.FindUserQuest(ctx, suid, failed.GetQuestId())
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatal(err)
	}
	if quest.GetLastClearedTime() != 0 || exerciseLesson(t, profileCall(t, client.Top, &papi.Empty{}), "exercise-basic-02").GetUnlocked() {
		t.Fatal("failed first lesson unlocked the next lesson")
	}

	// Final saved selections preceding successful captures 036, 043, 050,
	// 060, 070, 080, and 101. The progression order comes from master unlock
	// conditions, not lexical exercise IDs; later master additions may follow.
	lessons := []struct {
		id        string
		next      string
		positions []*papi.ExerciseDeckPosition
	}{
		{"exercise-basic-01", "exercise-basic-02", []*papi.ExerciseDeckPosition{
			{Position: 4, CardId: "card-ski-03-schl-00"}, {Position: 5, CardId: "card-chs-03-schl-00"},
		}},
		{"exercise-basic-02", "exercise-basic-03", []*papi.ExerciseDeckPosition{
			{Position: 1, CardId: "card-aoi-03-schl-00"},
		}},
		{"exercise-basic-03", "exercise-basic-05", []*papi.ExerciseDeckPosition{
			{Position: 1, CardId: "card-suz-03-schl-00"}, {Position: 2, CardId: "card-chs-03-schl-00"},
		}},
		{"exercise-basic-05", "exercise-basic-06", []*papi.ExerciseDeckPosition{
			{Position: 1, CardId: "card-rei-04-casl-00"}, {Position: 2, CardId: "card-ktn-04-casl-00"},
			{Position: 3, CardId: "card-ngs-04-casl-00"}, {Position: 4, CardId: "card-ski-04-casl-00"},
			{Position: 5, CardId: "card-skr-04-casl-00"},
		}},
		{"exercise-basic-06", "exercise-basic-07", []*papi.ExerciseDeckPosition{
			{Position: 1, CardId: "card-ktn-04-casl-00", Part1AccessoryId: "ac-1-visual-04"},
			{Position: 4, CardId: "card-suz-04-casl-00", Part1AccessoryId: "ac-1-vocal-04"},
		}},
		{"exercise-basic-07", "exercise-basic-04", []*papi.ExerciseDeckPosition{
			{Position: 3, CardId: "card-ktn-04-casl-00", PhotoAllInOneIds: []string{"phot-exercise-07-03", "phot-exercise-07-02"}},
			{Position: 4, CardId: "card-mei-04-casl-00", PhotoAllInOneIds: []string{"phot-exercise-07-01"}},
		}},
		{"exercise-basic-04", "", []*papi.ExerciseDeckPosition{
			{Position: 1, CardId: "card-skr-05-idol-00", Part1AccessoryId: "ac-1-vocal-05", PhotoAllInOneIds: []string{"phot-exercise-04-01"}},
			{Position: 3, CardId: "card-rio-04-casl-00", Part1AccessoryId: "ac-1-dance-05", PhotoAllInOneIds: []string{"phot-exercise-04-02"}},
			{Position: 5, CardId: "card-yu-05-idol-00", Part1AccessoryId: "ac-1-visual-05", PhotoAllInOneIds: []string{"phot-exercise-04-03"}},
		}},
	}
	for _, lesson := range lessons {
		if !exerciseLesson(t, profileCall(t, client.Top, &papi.Empty{}), lesson.id).GetUnlocked() {
			t.Fatalf("previous lesson did not unlock %s", lesson.id)
		}
		if lesson.id == "exercise-basic-04" {
			// Captures 092/094 put the third photo on Sakura instead of Yu.
			// Only correcting that distribution in 099 reaches the clear score.
			wrong := proto.Clone(&papi.ExerciseSaveDeckRequest{ExerciseId: lesson.id, Positions: lesson.positions}).(*papi.ExerciseSaveDeckRequest)
			wrong.Positions[0].PhotoAllInOneIds = append(wrong.Positions[0].PhotoAllInOneIds, wrong.Positions[2].PhotoAllInOneIds...)
			wrong.Positions[2].PhotoAllInOneIds = nil
			profileCall(t, client.SaveDeck, wrong)
			before := exerciseRewardAmounts(t)
			failed := profileCall(t, client.Start, &papi.ExerciseStartRequest{ExerciseId: lesson.id, IsSkip: true})
			var score int64
			if users := failed.GetResult().GetUserInfos(); len(users) > 0 {
				score = users[0].GetUserResult().GetScore()
			}
			if failed.GetResult() == nil || failed.GetResult().GetCleared() || len(failed.GetClearRewards()) != 0 {
				t.Fatalf("incorrect lesson 4 photo distribution passed or received a clear reward: cleared=%t score=%d", failed.GetResult().GetCleared(), score)
			}
			progress, err := opuser.FindUserQuest(ctx, suid, failed.GetQuestId())
			if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
				t.Fatal(err)
			}
			if progress.GetLastClearedTime() != 0 || !maps.Equal(before, exerciseRewardAmounts(t)) {
				t.Fatal("incorrect lesson 4 photo distribution advanced clear progress or granted rewards")
			}
		}
		saved := profileCall(t, client.SaveDeck, &papi.ExerciseSaveDeckRequest{ExerciseId: lesson.id, Positions: lesson.positions})
		if saved.GetCommonResponse() == nil || proto.Size(saved.GetCommonResponse().GetUpdatedData()) != 0 || proto.Size(saved.GetCommonResponse().GetDeletedData()) != 0 {
			t.Fatalf("%s SaveDeck changed normal user inventories", lesson.id)
		}
		stored := exerciseLesson(t, profileCall(t, client.Top, &papi.Empty{}), lesson.id)
		if len(stored.GetPositions()) != len(lesson.positions) {
			t.Fatalf("%s did not reload its editable rental slots", lesson.id)
		}
		for i, position := range lesson.positions {
			if !proto.Equal(position, stored.GetPositions()[i]) {
				t.Fatalf("%s did not persist editable slot %d", lesson.id, position.GetPosition())
			}
		}
		assets := profileCall(t, client.ListAsset, &papi.ExerciseListAssetRequest{ExerciseId: lesson.id})
		if len(assets.GetCharacterAssets()) != 5 {
			t.Fatalf("%s prefetch omitted rental or fixed slots", lesson.id)
		}
		itemsBefore := exerciseRewardAmounts(t)
		var newMusicID string
		if lesson.id == "exercise-basic-07" {
			exercise, err := opmaster.FindExercise(ctx, lesson.id)
			if err != nil {
				t.Fatal(err)
			}
			quest, err := opmaster.FindQuest(ctx, exercise.GetQuestId())
			if err != nil {
				t.Fatal(err)
			}
			newMusicID = quest.GetMusicId()
			musics, err := opuser.FindAllUserMusics(ctx, suid)
			if err != nil {
				t.Fatal(err)
			}
			for _, music := range musics {
				if music.GetMusicId() == newMusicID {
					t.Fatal("lesson 7 music-unlock fixture already owns the song")
				}
			}
		}
		request := &papi.ExerciseStartRequest{ExerciseId: lesson.id, IsSkip: true}
		started := profileCall(t, client.Start, request)
		var score int64
		if users := started.GetResult().GetUserInfos(); len(users) > 0 {
			score = users[0].GetUserResult().GetScore()
		}
		if !started.GetResult().GetCleared() || len(started.GetClearRewards()) == 0 || len(started.GetRankPatterns()) == 0 || len(started.GetResult().GetCharts()) == 0 {
			t.Fatalf("%s captured winning formation did not clear with rewards and a full trace: cleared=%t score=%d", lesson.id, started.GetResult().GetCleared(), score)
		}
		if len(started.GetResult().GetUserInfos()) == 0 || len(started.GetResult().GetUserInfos()[0].GetUserDeck().GetCards()) != 5 {
			t.Fatalf("%s omitted its rental live deck", lesson.id)
		}
		if newMusicID != "" {
			musics := started.GetCommonResponse().GetUpdatedData().GetMusics()
			if len(musics) != 1 || musics[0].GetMusicId() != newMusicID {
				t.Fatal("lesson 7 did not report its newly unlocked music")
			}
			stored, err := opuser.FindAllUserMusics(ctx, suid)
			if err != nil {
				t.Fatal(err)
			}
			var found bool
			for _, music := range stored {
				found = found || music.GetMusicId() == newMusicID
			}
			if !found {
				t.Fatal("lesson 7 music unlock was not persisted")
			}
		}
		for i, card := range started.GetResult().GetUserInfos()[0].GetUserDeck().GetCards() {
			asset := assets.GetCharacterAssets()[i]
			if card.GetDisplayCharacterId() != asset.GetCharacterId() || card.GetDisplayCostumeId() != asset.GetCostumeId() || card.GetDisplayHairId() != asset.GetHairId() {
				t.Fatalf("%s slot %d differs from its prefetched appearance", lesson.id, i+1)
			}
		}
		itemsAfter := exerciseRewardAmounts(t)
		for _, reward := range started.GetClearRewards() {
			key := reward.GetResourceId()
			if reward.GetResourceType() == penum.ResourceType_ResourceType_Stone {
				key = "free-stone"
			} else if reward.GetResourceType() != penum.ResourceType_ResourceType_Item {
				t.Fatalf("%s needs a new reward assertion for %v", lesson.id, reward.GetResourceType())
			}
			if itemsAfter[key]-itemsBefore[key] != reward.GetAmountI64() {
				t.Fatalf("%s listed a reward without crediting it", lesson.id)
			}
		}
		persisted, err := opuser.FindUserQuest(ctx, suid, started.GetQuestId())
		if err != nil || persisted.GetLastClearedTime() == 0 || persisted.GetHighestScore() != started.GetResult().GetUserInfos()[0].GetUserResult().GetScore() {
			t.Fatalf("%s clear did not persist quest progress: %v", lesson.id, err)
		}
		if lesson.next != "" && !exerciseLesson(t, profileCall(t, client.Top, &papi.Empty{}), lesson.next).GetUnlocked() {
			t.Fatalf("%s clear did not unlock %s", lesson.id, lesson.next)
		}
		// Exercise has fixed randomness: replaying the same formation has the
		// same simulation, but it must not receive the first-clear reward again.
		replayed := profileCall(t, client.Start, request)
		firstResult := proto.Clone(started.GetResult()).(*papi.LiveResult)
		replayResult := proto.Clone(replayed.GetResult()).(*papi.LiveResult)
		firstResult.DetailLogId, replayResult.DetailLogId = "", ""
		if !proto.Equal(firstResult, replayResult) || len(replayed.GetClearRewards()) != 0 {
			t.Fatalf("%s replay changed its fixed simulation or duplicated clear rewards", lesson.id)
		}
		if newMusicID != "" && len(replayed.GetCommonResponse().GetUpdatedData().GetMusics()) != 0 {
			t.Fatal("lesson 7 replay reported the music unlock again")
		}
		itemsReplayed := exerciseRewardAmounts(t)
		for key, amount := range itemsAfter {
			if itemsReplayed[key] != amount {
				t.Fatalf("%s replay credited %s again", lesson.id, key)
			}
		}
	}
	if !proto.Equal(inventoryBefore, exerciseInventorySnapshot(t)) {
		t.Fatal("rental exercise modified real cards, photos, accessories, characters, or saved decks")
	}
	userAfter, err := opuser.FindUser(ctx, suid)
	if err != nil || userAfter.GetManagerExp() != userBefore.GetManagerExp() {
		t.Fatalf("exercise unexpectedly awarded manager EXP: %v", err)
	}
}

func TestExerciseRentalDeckValidationScenario(t *testing.T) {
	setupExerciseTestUser(t)
	client := papi.NewExerciseClient(testConfig.GrpcConn)
	valid := &papi.ExerciseSaveDeckRequest{ExerciseId: "exercise-basic-01", Positions: []*papi.ExerciseDeckPosition{
		{Position: 4, CardId: "card-ski-03-schl-00"}, {Position: 5, CardId: "card-chs-03-schl-00"},
	}}
	// Editing accepts partial snapshots, but prefetch and simulation require
	// every lane (including the immutable master slots) to be filled.
	profileCall(t, client.SaveDeck, &papi.ExerciseSaveDeckRequest{ExerciseId: valid.GetExerciseId(), Positions: valid.Positions[:1]})
	profileReject(t, codes.FailedPrecondition, client.ListAsset, &papi.ExerciseListAssetRequest{ExerciseId: valid.GetExerciseId()})
	profileReject(t, codes.FailedPrecondition, client.Start, &papi.ExerciseStartRequest{ExerciseId: valid.GetExerciseId()})
	profileCall(t, client.SaveDeck, valid)
	before := profileCall(t, client.Top, &papi.Empty{})
	inventoryBefore := exerciseInventorySnapshot(t)
	profileReject(t, codes.InvalidArgument, client.SaveDeck, &papi.ExerciseSaveDeckRequest{})
	profileReject(t, codes.NotFound, client.SaveDeck, &papi.ExerciseSaveDeckRequest{ExerciseId: "exercise-does-not-exist"})
	profileReject(t, codes.InvalidArgument, client.Start, &papi.ExerciseStartRequest{})
	profileReject(t, codes.NotFound, client.Start, &papi.ExerciseStartRequest{ExerciseId: "exercise-does-not-exist"})
	profileReject(t, codes.InvalidArgument, client.ListAsset, &papi.ExerciseListAssetRequest{})
	profileReject(t, codes.NotFound, client.ListAsset, &papi.ExerciseListAssetRequest{ExerciseId: "exercise-does-not-exist"})
	profileReject(t, codes.FailedPrecondition, client.SaveDeck, &papi.ExerciseSaveDeckRequest{ExerciseId: "exercise-basic-02", Positions: []*papi.ExerciseDeckPosition{{Position: 1, CardId: "card-aoi-03-schl-00"}}})
	invalid := []struct {
		name string
		edit func(*papi.ExerciseSaveDeckRequest)
	}{
		{"fixed slot", func(r *papi.ExerciseSaveDeckRequest) { r.Positions[0].Position = 1 }},
		{"invalid slot", func(r *papi.ExerciseSaveDeckRequest) { r.Positions[0].Position = 6 }},
		{"duplicate slot", func(r *papi.ExerciseSaveDeckRequest) { r.Positions[1].Position = r.Positions[0].Position }},
		{"unknown card", func(r *papi.ExerciseSaveDeckRequest) { r.Positions[0].CardId = "card-does-not-exist" }},
		{"card outside rental pool", func(r *papi.ExerciseSaveDeckRequest) { r.Positions[0].CardId = "card-skr-05-idol-00" }},
		{"duplicate rental card", func(r *papi.ExerciseSaveDeckRequest) { r.Positions[1].CardId = r.Positions[0].CardId }},
		{"unknown accessory", func(r *papi.ExerciseSaveDeckRequest) { r.Positions[0].Part1AccessoryId = "ac-does-not-exist" }},
		{"accessory outside rental pool", func(r *papi.ExerciseSaveDeckRequest) { r.Positions[0].Part1AccessoryId = "ac-1-vocal-04" }},
		{"unknown photo", func(r *papi.ExerciseSaveDeckRequest) {
			r.Positions[0].PhotoAllInOneIds = []string{"phot-does-not-exist"}
		}},
		{"photo outside rental pool", func(r *papi.ExerciseSaveDeckRequest) {
			r.Positions[0].PhotoAllInOneIds = []string{"phot-exercise-07-01"}
		}},
	}
	for _, tc := range invalid {
		r := proto.Clone(valid).(*papi.ExerciseSaveDeckRequest)
		tc.edit(r)
		profileReject(t, codes.InvalidArgument, client.SaveDeck, r)
		if !proto.Equal(before, profileCall(t, client.Top, &papi.Empty{})) {
			t.Fatalf("rejected %s changed saved exercise selections", tc.name)
		}
	}
	if !proto.Equal(inventoryBefore, exerciseInventorySnapshot(t)) {
		t.Fatal("rejected exercise saves changed real user inventory")
	}
	profileCall(t, client.SaveDeck, &papi.ExerciseSaveDeckRequest{ExerciseId: valid.GetExerciseId()})
	if len(exerciseLesson(t, profileCall(t, client.Top, &papi.Empty{}), valid.GetExerciseId()).GetPositions()) != 0 {
		t.Fatal("an empty editable snapshot did not remove the previous selection")
	}
	profileReject(t, codes.FailedPrecondition, client.Start, &papi.ExerciseStartRequest{ExerciseId: valid.GetExerciseId()})
}

func TestExerciseDuplicateCharacterAppearanceScenario(t *testing.T) {
	setupExerciseTestUser(t)
	questSeedCleared(t, "qt-exercise-basic-11")
	client := papi.NewExerciseClient(testConfig.GrpcConn)
	inventoryBefore := exerciseInventorySnapshot(t)
	// Lesson 12 fixes another Chisa card in lane 3. Both scoring cards remain
	// valid, but the second occurrence needs a different visual performer.
	request := &papi.ExerciseSaveDeckRequest{ExerciseId: "exercise-basic-12", Positions: []*papi.ExerciseDeckPosition{
		{Position: 2, CardId: "card-chs-05-idol-00"},
		{Position: 4, CardId: "card-aoi-05-idol-00"},
		{Position: 5, CardId: "card-ngs-05-mizg-01"},
	}}
	profileCall(t, client.SaveDeck, request)
	prefetch := &papi.ExerciseListAssetRequest{ExerciseId: request.GetExerciseId()}
	response := profileCall(t, client.ListAsset, prefetch)
	assets := response.GetCharacterAssets()
	if len(assets) != 5 {
		t.Fatal("duplicate-character rental deck did not produce five appearances")
	}
	wanted := map[int]string{0: "char-ai", 1: "char-chs", 3: "char-aoi", 4: "char-ngs"}
	seen := map[string]bool{}
	for index, asset := range assets {
		if id := wanted[index]; id != "" && asset.GetCharacterId() != id {
			t.Fatalf("duplicate substitution stole the original character in slot %d", index+1)
		}
		if seen[asset.GetCharacterId()] || asset.GetCharacterId() == "" || asset.GetCostumeId() == "" || asset.GetHairId() == "" {
			t.Fatalf("exercise returned duplicate or incomplete appearance: %v", asset)
		}
		seen[asset.GetCharacterId()] = true
	}
	if !proto.Equal(response, profileCall(t, client.ListAsset, prefetch)) {
		t.Fatal("rental duplicate-character substitution was not deterministic")
	}
	stored := exerciseLesson(t, profileCall(t, client.Top, &papi.Empty{}), request.GetExerciseId())
	for index, position := range request.GetPositions() {
		if len(stored.GetPositions()) != len(request.GetPositions()) || !proto.Equal(stored.GetPositions()[index], position) {
			t.Fatal("cosmetic substitution altered saved rental scoring cards")
		}
	}
	if !proto.Equal(inventoryBefore, exerciseInventorySnapshot(t)) {
		t.Fatal("rental appearance substitution mutated the player's inventory")
	}
}

func TestExerciseCatalogSimulationScenario(t *testing.T) {
	setupExerciseTestUser(t)
	ctx, suid := context.Background(), testConfig.TestUserSuid
	client := papi.NewExerciseClient(testConfig.GrpcConn)
	rows, err := opmaster.FindAllExercises(ctx)
	if err != nil || len(rows) == 0 {
		t.Fatalf("Exercise master catalog is unavailable: %v", err)
	}
	// Unlock the lesson chain without god mode or changes to the real rental
	// inventories. This scenario exercises simulation, not first-clear rewards.
	for _, row := range rows {
		if err := opuser.UpsertUserQuest(ctx, suid, &ptransaction.UserQuest{QuestId: row.GetQuestId(), LastClearedTime: time.Now().UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	inventoryBefore := exerciseInventorySnapshot(t)
	rewardsBefore := exerciseRewardAmounts(t)
	var scoreLessons, battleLessons int
	for _, row := range rows {
		quest, err := opmaster.FindQuest(ctx, row.GetQuestId())
		if err != nil {
			t.Fatal(err)
		}
		reserved, usedCards := map[int32]bool{}, map[string]bool{}
		if row.GetExerciseDeckId() != "" {
			fixed, err := opmaster.FindExerciseDeck(ctx, row.GetExerciseDeckId())
			if err != nil {
				t.Fatal(err)
			}
			for _, position := range fixed.GetPositions() {
				reserved[position.GetPosition()] = true
				usedCards[position.GetCardId()] = true
			}
		}
		var cardIDs []string
		if row.GetExerciseCardGroupId() != "" {
			group, err := opmaster.FindExerciseCardGroup(ctx, row.GetExerciseCardGroupId())
			if err != nil {
				t.Fatal(err)
			}
			cardIDs = group.GetCardIds()
		}
		var positions []*papi.ExerciseDeckPosition
		for position := int32(1); position <= 5; position++ {
			if reserved[position] {
				continue
			}
			var selected string
			for _, cardID := range cardIDs {
				if !usedCards[cardID] {
					selected = cardID
					break
				}
			}
			if selected == "" {
				t.Fatalf("%s has no unique rental card for slot %d", row.GetId(), position)
			}
			usedCards[selected] = true
			positions = append(positions, &papi.ExerciseDeckPosition{Position: position, CardId: selected})
		}
		profileCall(t, client.SaveDeck, &papi.ExerciseSaveDeckRequest{ExerciseId: row.GetId(), Positions: positions})
		assets := profileCall(t, client.ListAsset, &papi.ExerciseListAssetRequest{ExerciseId: row.GetId()})
		if len(assets.GetCharacterAssets()) != 5 {
			t.Fatalf("%s catalog deck did not resolve five appearances", row.GetId())
		}
		request := &papi.ExerciseStartRequest{ExerciseId: row.GetId(), IsSkip: true}
		if quest.GetClearScore() == 0 {
			battleLessons++
			before, err := opuser.FindUserQuest(ctx, suid, row.GetQuestId())
			if err != nil {
				t.Fatal(err)
			}
			rpcCtx, cancel, err := getFullGrpcCtx()
			if err != nil {
				t.Fatal(err)
			}
			response := callGRPCWire(t, rpcCtx, papi.Exercise_Start_FullMethodName, request)
			cancel()
			assertGRPCWireAction(t, response, "title", "1003", "未実装の機能です")
			after, err := opuser.FindUserQuest(ctx, suid, row.GetQuestId())
			if err != nil || !proto.Equal(before, after) {
				t.Fatalf("unsupported battle %s mutated quest progress: %v", row.GetId(), err)
			}
			if !maps.Equal(rewardsBefore, exerciseRewardAmounts(t)) {
				t.Fatalf("unsupported battle %s granted a reward", row.GetId())
			}
			continue
		}
		scoreLessons++
		started := profileCall(t, client.Start, request)
		result := started.GetResult()
		if len(result.GetUserInfos()) != 1 || len(result.GetUserInfos()[0].GetUserDeck().GetCards()) != 5 || len(result.GetCharts()) == 0 || len(started.GetRankPatterns()) == 0 || len(started.GetClearRewards()) != 0 {
			t.Fatalf("%s catalog simulation omitted its trace or granted an already-claimed reward", row.GetId())
		}
		repeated := profileCall(t, client.Start, request)
		if !proto.Equal(result, repeated.GetResult()) || len(repeated.GetClearRewards()) != 0 {
			t.Fatalf("%s catalog simulation was not deterministic or rewarded a repeat", row.GetId())
		}
	}
	if !proto.Equal(inventoryBefore, exerciseInventorySnapshot(t)) || !maps.Equal(rewardsBefore, exerciseRewardAmounts(t)) {
		t.Fatal("catalog simulation changed user inventory or granted already-claimed rewards")
	}
	t.Logf("checked %d score lessons and %d unsupported battle guards", scoreLessons, battleLessons)
}

func setupExerciseTestUser(t *testing.T) {
	t.Helper()
	// Register before setupTestUser so its later-registered deletion runs first.
	t.Cleanup(func() {
		decks, err := opuser.FindAllUserExerciseDecks(context.Background(), testConfig.TestUserSuid)
		if err != nil || len(decks) != 0 {
			t.Errorf("deleting the configured test user left exercise decks behind: count=%d err=%v", len(decks), err)
		}
	})
	setupTestUser(t)
	ctx := context.Background()
	area, err := opmaster.FindArea(ctx, "area-exercise")
	if err != nil {
		t.Fatal(err)
	}
	areas, err := opmaster.FindAllAreas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{area.GetViewConditionId(), area.GetUnlockConditionId()} {
		if id == "" {
			continue
		}
		condition, err := opmaster.FindCondition(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, setting := range condition.GetSettings() {
			if setting.GetClearArea().GetAreaId() == "" {
				continue
			}
			required, err := opmaster.FindArea(ctx, setting.GetClearArea().GetAreaId())
			if err != nil {
				t.Fatal(err)
			}
			for _, candidate := range areas {
				if candidate.GetType() == penum.AreaType_AreaType_Main && candidate.GetOrder() <= required.GetOrder() {
					questSeedFullArea(t, candidate)
				}
			}
		}
	}
}

func exerciseLesson(t *testing.T, top *papi.ExerciseTopResponse, id string) *papi.ExerciseInfo {
	t.Helper()
	for _, lesson := range top.GetExercises() {
		if lesson.GetExerciseId() == id {
			return lesson
		}
	}
	t.Fatalf("Exercise.Top omitted %s", id)
	return nil
}

func exerciseInventorySnapshot(t *testing.T) *papi.UpdatedData {
	t.Helper()
	ctx, suid := context.Background(), testConfig.TestUserSuid
	result := &papi.UpdatedData{}
	var err error
	if result.Cards, err = opuser.FindAllUserCards(ctx, suid); err != nil {
		t.Fatal(err)
	}
	if result.Photos, err = opuser.FindAllUserPhotos(ctx, suid); err != nil {
		t.Fatal(err)
	}
	if result.Accessories, err = opuser.FindAllUserAccessories(ctx, suid); err != nil {
		t.Fatal(err)
	}
	if result.Characters, err = opuser.FindAllUserCharacters(ctx, suid); err != nil {
		t.Fatal(err)
	}
	if result.Decks, err = opuser.FindAllUserDecks(ctx, suid); err != nil {
		t.Fatal(err)
	}
	if result.DeckPositions, err = opuser.FindAllUserDeckPositions(ctx, suid); err != nil {
		t.Fatal(err)
	}
	return result
}

func exerciseRewardAmounts(t *testing.T) map[string]int64 {
	t.Helper()
	ctx, suid := context.Background(), testConfig.TestUserSuid
	items, err := opuser.FindAllUserItems(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	balance, err := opuser.FindUserBalance(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]int64{"free-stone": int64(balance.GetFreeBalance()), "paid-stone": int64(balance.GetPaidBalance())}
	for _, item := range items {
		result[item.GetItemId()] += item.GetAmount()
	}
	return result
}
