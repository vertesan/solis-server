package test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"google.golang.org/protobuf/proto"
)

func TestDeckEditingScenario(t *testing.T) {
	setupTestUser(t)
	fixture := prepareDeckFixture(t)
	ctx := context.Background()
	client := papi.NewDeckClient(testConfig.GrpcConn)
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	initialSequence := user.GetDeckSequence()
	initialDecks, err := opuser.FindAllUserDecks(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	var highestNumber int32
	for _, deck := range initialDecks {
		highestNumber = max(highestNumber, deck.GetNumber())
	}
	if initialSequence != int64(highestNumber) {
		t.Fatal("new user deckSequence is not the last allocated deck number")
	}
	before := deckInventory(t, fixture)
	position := &ptransaction.UserDeckPosition{
		Position: 1, CardId: fixture.cards[0].GetId(), CostumeId: fixture.costume.GetId(),
		Part1AccessoryId: fixture.accessories[0].GetId(), PhotoIds: []string{fixture.photos[0].GetPhotoId()},
	}
	request := &papi.DeckSaveRequest{Name: "編成テスト", UserDeckPositions: []*ptransaction.UserDeckPosition{position}, DeckEditType: 1}
	created := profileCall(t, client.Save, request)
	number := created.GetCommonResponse().GetUpdatedData().GetDecks()[0].GetNumber()
	if int64(number) != initialSequence+1 || created.GetCommonResponse().GetUpdatedData().GetUser().GetDeckSequence() != int64(number) {
		t.Fatal("new deck did not allocate and return the next sequence")
	}
	first := proto.Clone(position).(*ptransaction.UserDeckPosition)
	first.Number, first.HairId = number, fixture.hairID
	assertDeckState(t, number, request.GetName(), []*ptransaction.UserDeckPosition{first})
	assertDeckDelta(t, created.GetCommonResponse(), number, request.GetName(), []*ptransaction.UserDeckPosition{first})

	// A second card of the same idol is legal, but only the first appearance
	// receives its requested costume. Accessories are counted within each deck.
	second := &ptransaction.UserDeckPosition{
		Position: 2, CardId: fixture.cards[1].GetId(), CostumeId: fixture.costume.GetId(),
		Part1AccessoryId: fixture.accessories[0].GetId(), Part2AccessoryId: fixture.accessories[1].GetId(),
		PhotoIds: []string{fixture.photos[1].GetPhotoId()},
	}
	request.Number, request.UserDeckPositions = number, []*ptransaction.UserDeckPosition{position, second}
	expanded := profileCall(t, client.Save, request)
	secondExpected := proto.Clone(second).(*ptransaction.UserDeckPosition)
	secondExpected.Number, secondExpected.CostumeId = number, ""
	assertDeckDelta(t, expanded.GetCommonResponse(), number, request.GetName(), []*ptransaction.UserDeckPosition{first, secondExpected})
	assertDeckState(t, number, request.GetName(), []*ptransaction.UserDeckPosition{first, secondExpected})

	// Copying a unit does not globally reserve its photos or accessories.
	copyRequest := proto.Clone(request).(*papi.DeckSaveRequest)
	copyRequest.Number, copyRequest.Name = 0, "コピー"
	copyResponse := profileCall(t, client.Save, copyRequest)
	copyNumber := copyResponse.GetCommonResponse().GetUpdatedData().GetDecks()[0].GetNumber()
	if copyNumber != number+1 {
		t.Fatal("copied deck did not allocate a fresh sequence")
	}

	// Position is the slot key, not the request slice index. Replacing the
	// first idol keeps exactly the equipment the client carries forward.
	position.Position, position.CardId, position.CostumeId = 5, fixture.cards[2].GetId(), ""
	second.Position, second.CostumeId = 2, ""
	request.Name, request.UserDeckPositions = "並び替え", []*ptransaction.UserDeckPosition{position, second}
	reordered := profileCall(t, client.Save, request)
	first = proto.Clone(position).(*ptransaction.UserDeckPosition)
	first.Number = number
	secondExpected = proto.Clone(second).(*ptransaction.UserDeckPosition)
	secondExpected.Number = number
	assertDeckDelta(t, reordered.GetCommonResponse(), number, request.GetName(), []*ptransaction.UserDeckPosition{first, secondExpected})
	if !deckDeletedPosition(reordered.GetCommonResponse(), number, 1) {
		t.Fatal("moving a position omitted the old slot from deletedData")
	}
	assertDeckState(t, number, request.GetName(), []*ptransaction.UserDeckPosition{first, secondExpected})

	// Gear clearing and omitted-position removal are full replacements.
	position.Part1AccessoryId, position.PhotoIds = "", nil
	request.UserDeckPositions, request.DeckEditType = []*ptransaction.UserDeckPosition{position}, 0
	cleared := profileCall(t, client.Save, request)
	first = proto.Clone(position).(*ptransaction.UserDeckPosition)
	first.Number = number
	assertDeckState(t, number, request.GetName(), []*ptransaction.UserDeckPosition{first})
	if !deckDeletedPosition(cleared.GetCommonResponse(), number, 2) {
		t.Fatal("omitted occupied position was not deleted")
	}
	request.UserDeckPositions = nil
	empty := profileCall(t, client.Save, request)
	if !deckDeletedPosition(empty.GetCommonResponse(), number, 5) {
		t.Fatal("clearing a deck omitted its final occupied slot")
	}
	assertDeckState(t, number, request.GetName(), nil)

	deleted := profileCall(t, client.Delete, &papi.DeckDeleteRequest{Number: copyNumber})
	if len(deleted.GetCommonResponse().GetDeletedData().GetDecks()) != 1 || deleted.GetCommonResponse().GetDeletedData().GetDecks()[0].GetNumber() != copyNumber ||
		!deckDeletedPosition(deleted.GetCommonResponse(), copyNumber, 1) || !deckDeletedPosition(deleted.GetCommonResponse(), copyNumber, 2) {
		t.Fatal("deck deletion omitted deck or occupied-slot keys")
	}
	for _, deletedPosition := range deleted.GetCommonResponse().GetDeletedData().GetDeckPositions() {
		if !proto.Equal(deletedPosition, &ptransaction.UserDeckPosition{Number: copyNumber, Position: deletedPosition.GetPosition()}) {
			t.Fatal("deleted position should contain only its key")
		}
	}
	profileCall(t, client.Delete, &papi.DeckDeleteRequest{Number: copyNumber})
	profileCall(t, client.Delete, &papi.DeckDeleteRequest{Number: number})
	userGet := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	for _, deck := range userGet.GetDecks() {
		if deck.GetNumber() == number || deck.GetNumber() == copyNumber {
			t.Fatal("User.Get returned a deleted deck")
		}
	}
	for _, pos := range userGet.GetDeckPositions() {
		if pos.GetNumber() == number || pos.GetNumber() == copyNumber {
			t.Fatal("User.Get returned an orphaned deck position")
		}
	}
	if userGet.GetUser().GetDeckSequence() != int64(copyNumber) {
		t.Fatal("deletion rewound the deck sequence")
	}
	newDeck := profileCall(t, client.Save, &papi.DeckSaveRequest{Name: "再作成", DeckEditType: 2})
	if newDeck.GetCommonResponse().GetUpdatedData().GetDecks()[0].GetNumber() != copyNumber+1 {
		t.Fatal("new deck reused a deleted number")
	}
	profileCall(t, papi.NewProfileClient(testConfig.GrpcConn).UpdateBuddyCard, &papi.ProfileUpdateBuddyCardRequest{CardId: fixture.cards[0].GetId()})
	buddies := profileCall(t, client.BuddyList, &papi.Empty{})
	if len(buddies.GetFriendBuddyUserCardInfos()) != 0 || len(buddies.GetNotFriendBuddyUserCardInfos()) != 0 {
		t.Fatal("private-server buddy listing exposed another manager or self as a friend")
	}
	assertDeckInventory(t, before, deckInventory(t, fixture))
}

func TestDeckValidationAndCapacityScenario(t *testing.T) {
	setupTestUser(t)
	fixture := prepareDeckFixture(t)
	ctx := context.Background()
	client := papi.NewDeckClient(testConfig.GrpcConn)
	base := &papi.DeckSaveRequest{Name: "検証", DeckEditType: 1, UserDeckPositions: []*ptransaction.UserDeckPosition{{Position: 1, CardId: fixture.cards[0].GetId()}}}
	created := profileCall(t, client.Save, base)
	number := created.GetCommonResponse().GetUpdatedData().GetDecks()[0].GetNumber()
	base.Number = number
	beforeUser, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	beforeInventory := deckInventory(t, fixture)
	invalid := []struct {
		name string
		edit func(*papi.DeckSaveRequest)
	}{
		{"negative number", func(r *papi.DeckSaveRequest) { r.Number = -1 }},
		{"missing deck", func(r *papi.DeckSaveRequest) { r.Number = number + 1000 }},
		{"out of range position", func(r *papi.DeckSaveRequest) { r.UserDeckPositions[0].Position = 6 }},
		{"duplicate position", func(r *papi.DeckSaveRequest) {
			r.UserDeckPositions = append(r.UserDeckPositions, &ptransaction.UserDeckPosition{Position: 1, CardId: fixture.cards[1].GetId()})
		}},
		{"duplicate card", func(r *papi.DeckSaveRequest) {
			r.UserDeckPositions = append(r.UserDeckPositions, &ptransaction.UserDeckPosition{Position: 2, CardId: fixture.cards[0].GetId()})
		}},
		{"unowned card", func(r *papi.DeckSaveRequest) { r.UserDeckPositions[0].CardId = "card-deck-not-owned" }},
		{"unowned accessory", func(r *papi.DeckSaveRequest) { r.UserDeckPositions[0].Part1AccessoryId = "ac-deck-not-owned" }},
		{"wrong accessory slot", func(r *papi.DeckSaveRequest) {
			r.UserDeckPositions[0].Part1AccessoryId = fixture.accessories[1].GetId()
		}},
		{"wrong accessory wearer", func(r *papi.DeckSaveRequest) {
			r.UserDeckPositions[0].Part1AccessoryId = fixture.restrictedAccessory.GetId()
		}},
		{"unowned photo", func(r *papi.DeckSaveRequest) { r.UserDeckPositions[0].PhotoIds = []string{"photo-deck-not-owned"} }},
		{"duplicate photo", func(r *papi.DeckSaveRequest) {
			r.UserDeckPositions[0].PhotoIds = []string{fixture.photos[0].GetPhotoId(), fixture.photos[0].GetPhotoId()}
		}},
		{"photo used twice in deck", func(r *papi.DeckSaveRequest) {
			r.UserDeckPositions[0].PhotoIds = []string{fixture.photos[0].GetPhotoId()}
			r.UserDeckPositions = append(r.UserDeckPositions, &ptransaction.UserDeckPosition{Position: 2, CardId: fixture.cards[1].GetId(), PhotoIds: []string{fixture.photos[0].GetPhotoId()}})
		}},
		{"photo capacity", func(r *papi.DeckSaveRequest) {
			r.UserDeckPositions[0].PhotoIds = slices.Clone(fixture.overflowPhotoIDs)
		}},
		{"wrong photo wearer", func(r *papi.DeckSaveRequest) {
			r.UserDeckPositions[0].PhotoIds = []string{fixture.photos[5].GetPhotoId()}
		}},
		{"two retouched photos", func(r *papi.DeckSaveRequest) {
			r.UserDeckPositions[0].PhotoIds = []string{fixture.photos[6].GetPhotoId(), fixture.photos[7].GetPhotoId()}
		}},
		{"unowned costume", func(r *papi.DeckSaveRequest) { r.UserDeckPositions[0].CostumeId = "cos-deck-not-owned" }},
		{"wrong costume wearer", func(r *papi.DeckSaveRequest) {
			r.UserDeckPositions[0].CardId, r.UserDeckPositions[0].CostumeId = fixture.cards[2].GetId(), fixture.costume.GetId()
		}},
	}
	for _, testcase := range invalid {
		t.Run(testcase.name, func(t *testing.T) {
			request := proto.Clone(base).(*papi.DeckSaveRequest)
			testcase.edit(request)
			if _, err := profileInvoke(t, client.Save, request); err == nil {
				t.Fatalf("invalid deck was accepted: %v", request)
			}
			assertDeckState(t, number, base.GetName(), []*ptransaction.UserDeckPosition{{Number: number, Position: 1, CardId: fixture.cards[0].GetId()}})
		})
	}
	// A failed creation must not leak the allocated sequence or a partial deck.
	failedCreate := proto.Clone(base).(*papi.DeckSaveRequest)
	failedCreate.Number, failedCreate.UserDeckPositions[0].CardId = 0, "card-deck-not-owned"
	if _, err := profileInvoke(t, client.Save, failedCreate); err == nil {
		t.Fatal("invalid new deck was accepted")
	}
	afterUser, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil || !proto.Equal(beforeUser, afterUser) {
		t.Fatalf("rejected deck mutation changed the user: %v", err)
	}
	// Two available copies can be equipped twice; a third reference is not
	// allowed, but copies in a different saved unit are independent.
	tooManyAccessories := proto.Clone(base).(*papi.DeckSaveRequest)
	for i, card := range fixture.cards {
		if i == 0 {
			tooManyAccessories.UserDeckPositions[0].Part1AccessoryId = fixture.accessories[0].GetId()
			continue
		}
		tooManyAccessories.UserDeckPositions = append(tooManyAccessories.UserDeckPositions, &ptransaction.UserDeckPosition{Position: int32(i + 1), CardId: card.GetId(), Part1AccessoryId: fixture.accessories[0].GetId()})
	}
	if _, err := profileInvoke(t, client.Save, tooManyAccessories); err == nil {
		t.Fatal("deck used more accessory copies than owned")
	}
	assertDeckInventory(t, beforeInventory, deckInventory(t, fixture))

	// Concurrent creations serialize on the owner's sequence and cannot
	// replace one another or assign the same number.
	type createResult struct {
		response *papi.DeckSaveResponse
		err      error
	}
	results := make(chan createResult, 3)
	for index := range 3 {
		callCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			defer cancel()
			response, err := client.Save(callCtx, &papi.DeckSaveRequest{Name: fmt.Sprintf("並行%d", index), DeckEditType: 1})
			results <- createResult{response, err}
		}()
	}
	newNumbers := make(map[int32]bool)
	for range 3 {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent deck create: %v", result.err)
		}
		allocated := result.response.GetCommonResponse().GetUpdatedData().GetDecks()[0].GetNumber()
		if allocated <= number || newNumbers[allocated] {
			t.Fatal("concurrent deck creation reused a number")
		}
		newNumbers[allocated] = true
	}
	afterUser, err = opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil || afterUser.GetDeckSequence() != beforeUser.GetDeckSequence()+3 {
		t.Fatalf("concurrent creations lost a sequence increment: %v", err)
	}
	// Seed the remaining slots with large IDs: the cap is saved deck count,
	// not the value of a monotonically increasing deck number.
	settingID, err := config.GetGameSettingId()
	if err != nil {
		t.Fatal(err)
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		t.Fatal(err)
	}
	decks, err := opuser.FindAllUserDecks(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for len(decks) < int(setting.GetDeckCountMax()) {
		deck := &ptransaction.UserDeck{Number: int32(1000 + len(decks)), Name: "上限用"}
		if err := opuser.UpsertUserDeck(ctx, testConfig.TestUserSuid, deck); err != nil {
			t.Fatal(err)
		}
		decks = append(decks, deck)
	}
	if _, err := profileInvoke(t, client.Save, &papi.DeckSaveRequest{Name: "満員"}); err == nil {
		t.Fatal("new deck exceeded master deck count cap")
	}
	profileCall(t, client.Save, base) // Existing decks remain editable at capacity.
	profileCall(t, client.Delete, &papi.DeckDeleteRequest{Number: number})
	replacement := profileCall(t, client.Save, &papi.DeckSaveRequest{Name: "空いた枠"})
	if replacement.GetCommonResponse().GetUpdatedData().GetDecks()[0].GetNumber() <= number {
		t.Fatal("capacity release reused a deleted deck number")
	}
	assertDeckInventory(t, beforeInventory, deckInventory(t, fixture))
}

type deckFixture struct {
	cards               []*pmaster.Card
	accessories         []*pmaster.Accessory
	restrictedAccessory *pmaster.Accessory
	costume             *pmaster.Costume
	hairID              string
	photos              []*ptransaction.UserPhoto
	photoCapacity       int32
	overflowPhotoIDs    []string
}

func prepareDeckFixture(t *testing.T) deckFixture {
	t.Helper()
	promoteTestUser(t)
	ctx := context.Background()
	masters, err := opmaster.FindAllCards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	photoCapacities := make(map[string]int32)
	for _, card := range owned {
		photoCapacities[card.GetCardId()] = card.GetPhotoEquipableCount()
	}
	fixture := deckFixture{}
	byCharacter := make(map[string][]*pmaster.Card)
	for _, card := range masters {
		if photoCapacities[card.GetId()] >= 2 {
			byCharacter[card.GetCharacterId()] = append(byCharacter[card.GetCharacterId()], card)
		}
	}
	for _, card := range masters {
		if len(byCharacter[card.GetCharacterId()]) >= 2 {
			fixture.cards = append(fixture.cards, byCharacter[card.GetCharacterId()][:2]...)
			break
		}
	}
	if len(fixture.cards) != 2 {
		t.Fatal("master has no two owned photo-capable cards of one character")
	}
	fixture.photoCapacity = photoCapacities[fixture.cards[0].GetId()]
	for _, card := range masters {
		if photoCapacities[card.GetId()] >= 2 && card.GetCharacterId() != fixture.cards[0].GetCharacterId() {
			fixture.cards = append(fixture.cards, card)
			break
		}
	}
	if len(fixture.cards) != 3 {
		t.Fatal("master lacks a different character for deck fixture")
	}
	accessories, err := opmaster.FindAllAccessories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range []penum.AccessoryCategoryType{1, 2} {
		for _, accessory := range accessories {
			if accessory.GetCategoryType() == category && accessory.GetCharacterId() == "" && accessory.GetCharacterGroupId() == "" {
				fixture.accessories = append(fixture.accessories, accessory)
				if err := opuser.UpsertUserAccessory(ctx, testConfig.TestUserSuid, &ptransaction.UserAccessory{AccessoryId: accessory.GetId(), Amount: 2}); err != nil {
					t.Fatal(err)
				}
				break
			}
		}
	}
	if len(fixture.accessories) != 2 {
		t.Fatal("master lacks generic accessories for both parts")
	}
	for _, accessory := range accessories {
		if accessory.GetCategoryType() == penum.AccessoryCategoryType_AccessoryCategoryType_Part1 &&
			accessory.GetCharacterId() != "" && accessory.GetCharacterId() != fixture.cards[0].GetCharacterId() {
			if _, err := opuser.FindUserAccessory(ctx, testConfig.TestUserSuid, accessory.GetId()); err == nil {
				fixture.restrictedAccessory = accessory
				break
			}
		}
	}
	if fixture.restrictedAccessory == nil {
		t.Fatal("master lacks a character-restricted accessory fixture")
	}
	costumes, err := opmaster.FindAllCostumes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, costume := range costumes {
		if costume.GetCharacterId() == fixture.cards[0].GetCharacterId() && costume.GetDefaultHairId() != "" && !strings.HasPrefix(costume.GetCostumeTypeId(), "sd-") && !costume.GetIsUserUnavailable() {
			if _, err := opuser.FindUserCostume(ctx, testConfig.TestUserSuid, costume.GetId()); err == nil {
				fixture.costume = costume
				break
			}
		}
	}
	if fixture.costume == nil {
		t.Fatal("master lacks owned 3D costume for deck fixture")
	}
	hairs, err := opmaster.FindAllHairs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fixture.hairID = fixture.costume.GetDefaultHairId()
	for _, hair := range hairs {
		if hair.GetCharacterId() == fixture.cards[0].GetCharacterId() && hair.GetId() != fixture.costume.GetDefaultHairId() &&
			!slices.Contains(hair.GetNotWearableCostumeIds(), fixture.costume.GetId()) && (len(hair.GetWearableCostumeIds()) == 0 || slices.Contains(hair.GetWearableCostumeIds(), fixture.costume.GetId())) {
			if _, err := opuser.FindUserHair(ctx, testConfig.TestUserSuid, hair.GetId()); err == nil {
				fixture.hairID = hair.GetId()
				break
			}
		}
	}
	character, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, fixture.cards[0].GetCharacterId())
	if err != nil {
		t.Fatal(err)
	}
	character.LiveCostumeId, character.LiveHairId = fixture.costume.GetId(), fixture.hairID
	if err := opuser.UpsertUserCharacter(ctx, testConfig.TestUserSuid, character); err != nil {
		t.Fatal(err)
	}
	recipes, err := opmaster.FindAllPhotoRecipes(ctx)
	if err != nil || len(recipes) == 0 {
		t.Fatalf("load photo recipe fixture: %v", err)
	}
	for index := range 8 {
		photo := &ptransaction.UserPhoto{PhotoId: fmt.Sprintf("deck-scenario-photo-%d", index), Level: 1, Rarity: 1}
		if index == 5 {
			photo.CharacterIds = []string{fixture.cards[2].GetCharacterId()}
		}
		if index >= 6 {
			photo.PhotoRecipeId = recipes[0].GetId()
		}
		if err := opuser.UpsertUserPhoto(ctx, testConfig.TestUserSuid, photo); err != nil {
			t.Fatal(err)
		}
		fixture.photos = append(fixture.photos, photo)
	}
	// Keep the capacity case independent of wearer/recipe restrictions and
	// derive its size from the selected owned card rather than a fixed limit.
	for index := range fixture.photoCapacity + 1 {
		photo := &ptransaction.UserPhoto{PhotoId: fmt.Sprintf("deck-capacity-photo-%d", index), Level: 1, Rarity: 1}
		if err := opuser.UpsertUserPhoto(ctx, testConfig.TestUserSuid, photo); err != nil {
			t.Fatal(err)
		}
		fixture.overflowPhotoIDs = append(fixture.overflowPhotoIDs, photo.GetPhotoId())
		fixture.photos = append(fixture.photos, photo)
	}
	return fixture
}

func assertDeckState(t *testing.T, number int32, name string, expected []*ptransaction.UserDeckPosition) {
	t.Helper()
	response := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	var deck *ptransaction.UserDeck
	var positions []*ptransaction.UserDeckPosition
	for _, value := range response.GetDecks() {
		if value.GetNumber() == number {
			deck = value
		}
	}
	for _, value := range response.GetDeckPositions() {
		if value.GetNumber() == number {
			positions = append(positions, value)
		}
	}
	assertDeckDelta(t, &papi.CommonResponse{UpdatedData: &papi.UpdatedData{Decks: []*ptransaction.UserDeck{deck}, DeckPositions: positions}}, number, name, expected)
}

func assertDeckDelta(t *testing.T, response *papi.CommonResponse, number int32, name string, expected []*ptransaction.UserDeckPosition) {
	t.Helper()
	if len(response.GetUpdatedData().GetDecks()) != 1 || !proto.Equal(response.GetUpdatedData().GetDecks()[0], &ptransaction.UserDeck{Number: number, Name: name}) {
		t.Fatal("deck response lost its number or name")
	}
	actual := response.GetUpdatedData().GetDeckPositions()
	if len(actual) != len(expected) {
		t.Fatalf("deck has %d positions, want %d", len(actual), len(expected))
	}
	for _, want := range expected {
		found := false
		for _, got := range actual {
			if proto.Equal(got, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("deck position missing or incorrect: want %v; got %v", want, actual)
		}
	}
}

func deckDeletedPosition(response *papi.CommonResponse, number, position int32) bool {
	for _, value := range response.GetDeletedData().GetDeckPositions() {
		if value.GetNumber() == number && value.GetPosition() == position {
			return true
		}
	}
	return false
}

func deckInventory(t *testing.T, fixture deckFixture) map[string]proto.Message {
	t.Helper()
	ctx := context.Background()
	result := make(map[string]proto.Message)
	balance, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	result["balance"] = balance
	for _, master := range fixture.cards {
		card, err := opuser.FindUserCard(ctx, testConfig.TestUserSuid, master.GetId())
		if err != nil {
			t.Fatal(err)
		}
		result[master.GetId()] = card
	}
	for _, master := range fixture.accessories {
		accessory, err := opuser.FindUserAccessory(ctx, testConfig.TestUserSuid, master.GetId())
		if err != nil {
			t.Fatal(err)
		}
		result[master.GetId()] = accessory
	}
	for _, fixturePhoto := range fixture.photos {
		photo, err := opuser.FindUserPhoto(ctx, testConfig.TestUserSuid, fixturePhoto.GetPhotoId())
		if err != nil {
			t.Fatal(err)
		}
		result[fixturePhoto.GetPhotoId()] = photo
	}
	return result
}

func assertDeckInventory(t *testing.T, before, after map[string]proto.Message) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatal("deck operation changed inventory collection size")
	}
	for key, expected := range before {
		if !proto.Equal(expected, after[key]) {
			t.Fatalf("deck operation consumed or mutated %s", key)
		}
	}
}
