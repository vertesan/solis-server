package test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestProfileCustomizationScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	ctx := context.Background()
	client := papi.NewProfileClient(testConfig.GrpcConn)
	expectedUser, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := opuser.FindUserProfile(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil || len(cards) == 0 {
		t.Fatalf("load owned cards: %v", err)
	}
	characters, err := opuser.FindAllUserCharacters(ctx, testConfig.TestUserSuid)
	if err != nil || len(characters) < 3 {
		t.Fatalf("load owned characters: %v", err)
	}
	decorations, err := opuser.FindAllUserDecorations(ctx, testConfig.TestUserSuid)
	if err != nil || len(decorations) == 0 {
		t.Fatalf("load owned decorations: %v", err)
	}
	emblems, err := opuser.FindAllUserEmblems(ctx, testConfig.TestUserSuid)
	if err != nil || len(emblems) == 0 {
		t.Fatalf("load owned emblems: %v", err)
	}
	missionIDs := activateProfileMissions(t)
	balanceBefore, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}

	// Count characters, not UTF-8 bytes, and keep unrelated user/profile fields.
	expectedUser.Name = strings.Repeat("あ", 10)
	name := profileCall(t, client.UpdateName, &papi.ProfileUpdateNameRequest{Name: expectedUser.Name})
	if !proto.Equal(name.GetCommonResponse().GetUpdatedData().GetUser(), expectedUser) {
		t.Fatal("name update changed unrelated user fields or omitted the updated user")
	}
	profileReject(t, codes.InvalidArgument, client.UpdateName, &papi.ProfileUpdateNameRequest{Name: strings.Repeat("あ", 11)})
	profileReject(t, codes.InvalidArgument, client.UpdateName, &papi.ProfileUpdateNameRequest{Name: ""})

	expected.Message = strings.Repeat("猫", 60)
	message := profileCall(t, client.UpdateMessage, &papi.ProfileUpdateMessageRequest{Message: expected.Message})
	assertProfileDelta(t, message.GetCommonResponse(), expected)
	profileReject(t, codes.InvalidArgument, client.UpdateMessage, &papi.ProfileUpdateMessageRequest{Message: strings.Repeat("猫", 61)})
	expected.Message = ""
	assertProfileDelta(t, profileCall(t, client.UpdateMessage, &papi.ProfileUpdateMessageRequest{}).GetCommonResponse(), expected)

	expected.FavoriteCharacterIds = []string{characters[0].GetCharacterId(), characters[1].GetCharacterId(), characters[2].GetCharacterId()}
	favorites := profileCall(t, client.UpdateFavoriteCharacters, &papi.ProfileUpdateFavoriteCharactersRequest{FavoriteCharacterIds: expected.FavoriteCharacterIds})
	assertProfileDelta(t, favorites.GetCommonResponse(), expected)
	profileReject(t, codes.InvalidArgument, client.UpdateFavoriteCharacters, &papi.ProfileUpdateFavoriteCharactersRequest{FavoriteCharacterIds: []string{characters[0].GetCharacterId(), characters[0].GetCharacterId()}})
	profileReject(t, codes.NotFound, client.UpdateFavoriteCharacters, &papi.ProfileUpdateFavoriteCharactersRequest{FavoriteCharacterIds: []string{"char-profile-not-owned"}})

	// These enum values are taken from the captured layout request22.
	expected.LayoutType, expected.InformationType, expected.ColorType = 4, 3, 2
	layout := profileCall(t, client.UpdateLayout, &papi.ProfileUpdateLayoutRequest{LayoutType: expected.LayoutType, InformationType: expected.InformationType, ColorType: expected.ColorType})
	assertProfileDelta(t, layout.GetCommonResponse(), expected)
	profileReject(t, codes.InvalidArgument, client.UpdateLayout, &papi.ProfileUpdateLayoutRequest{LayoutType: 999, InformationType: 3, ColorType: 2})

	expected.DecorationId = decorations[0].GetDecorationId()
	decoration := profileCall(t, client.UpdateDecoration, &papi.ProfileUpdateDecorationRequest{DecorationId: expected.DecorationId})
	assertProfileDelta(t, decoration.GetCommonResponse(), expected)
	profileReject(t, codes.NotFound, client.UpdateDecoration, &papi.ProfileUpdateDecorationRequest{DecorationId: "deco-profile-not-owned"})
	expectedUser.EmblemId = emblems[0].GetEmblemId()
	emblem := profileCall(t, client.UpdateEmblem, &papi.ProfileUpdateEmblemRequest{EmblemId: expectedUser.EmblemId})
	if !proto.Equal(emblem.GetCommonResponse().GetUpdatedData().GetUser(), expectedUser) {
		t.Fatal("emblem update changed unrelated user fields")
	}
	profileReject(t, codes.NotFound, client.UpdateEmblem, &papi.ProfileUpdateEmblemRequest{EmblemId: "emb-profile-not-owned"})

	expectedUser.BuddyCardId = cards[0].GetCardId()
	buddy := profileCall(t, client.UpdateBuddyCard, &papi.ProfileUpdateBuddyCardRequest{CardId: expectedUser.BuddyCardId})
	if !proto.Equal(buddy.GetCommonResponse().GetUpdatedData().GetUser(), expectedUser) {
		t.Fatal("buddy selection changed unrelated user fields")
	}
	profileReject(t, codes.NotFound, client.UpdateBuddyCard, &papi.ProfileUpdateBuddyCardRequest{CardId: "card-profile-not-owned"})
	expected.BackgroundType = penum.ProfileBackgroundType_ProfileBackgroundType_Card
	expected.FavoriteCardId = cards[0].GetCardId()
	expected.FavoritePhotoId = ""
	expected.FavoriteCardDisplayType = 0 // Official profile requests use automatic/default display.
	background := profileCall(t, client.UpdateBackground, &papi.ProfileUpdateBackgroundRequest{BackgroundType: expected.BackgroundType, FavoriteCardId: expected.FavoriteCardId})
	assertProfileDelta(t, background.GetCommonResponse(), expected)
	expected.FavoriteCardDisplayType = penum.CardDisplayType_CardDisplayType_BeforeLimitBreak
	explicitBackground := profileCall(t, client.UpdateBackground, &papi.ProfileUpdateBackgroundRequest{
		BackgroundType: expected.BackgroundType, FavoriteCardId: expected.FavoriteCardId, FavoriteCardDisplayType: expected.FavoriteCardDisplayType,
	})
	assertProfileDelta(t, explicitBackground.GetCommonResponse(), expected)
	profileReject(t, codes.NotFound, client.UpdateBackground, &papi.ProfileUpdateBackgroundRequest{BackgroundType: expected.BackgroundType, FavoriteCardId: "card-profile-not-owned"})
	profileReject(t, codes.InvalidArgument, client.UpdateBackground, &papi.ProfileUpdateBackgroundRequest{BackgroundType: expected.BackgroundType, FavoriteCardId: expected.FavoriteCardId, FavoriteCardDisplayType: 999})
	storedCard, err := opuser.FindUserCard(ctx, testConfig.TestUserSuid, cards[0].GetCardId())
	if err != nil || !proto.Equal(storedCard, cards[0]) {
		t.Fatalf("profile background selection modified the owned card: %v", err)
	}

	for _, userID := range []string{testConfig.TestUserPublicId, testConfig.TestUserSuid} {
		self := profileCall(t, client.FindUser, &papi.ProfileFindUserRequest{UserId: userID})
		if self.GetUserId() != testConfig.TestUserSuid || self.GetName() != expectedUser.GetName() ||
			self.GetEmblemId() != expectedUser.GetEmblemId() || self.GetMessage() != expected.GetMessage() ||
			!slices.Equal(self.GetFavoriteCharacterIds(), expected.GetFavoriteCharacterIds()) ||
			self.GetFavoriteCard().GetCardId() != expected.GetFavoriteCardId() || self.GetFavoritePhoto() != nil ||
			self.GetFavoriteCard().GetDisplayType() != expected.GetFavoriteCardDisplayType() ||
			self.GetLayoutType() != expected.GetLayoutType() || self.GetDecorationId() != expected.GetDecorationId() {
			t.Fatalf("self lookup does not reflect customized profile: %v", self)
		}
	}
	profileReject(t, codes.NotFound, client.FindUser, &papi.ProfileFindUserRequest{UserId: "ZZZZZZZZ"})
	top := profileCall(t, client.Top, &papi.Empty{})
	if len(top.GetFriendProfiles()) != 0 || top.GetHierarchyCurrentRank() != 1 || top.GetHierarchyCurrentDetailGradeId() == "" {
		t.Fatalf("Profile.Top has inconsistent private-server hierarchy/social state: %v", top)
	}
	assertProfileHierarchyPreview(t, expected, expectedUser.GetBuddyCardId())

	profileCall(t, client.ClearTwitterMission, &papi.Empty{})
	profileCall(t, client.ClearTwitterMission, &papi.Empty{})
	assertProfileMissionProgress(t, missionIDs)
	balanceAfter, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	if err != nil || !proto.Equal(balanceBefore, balanceAfter) {
		t.Fatalf("profile edits or mission completion directly granted/spent currency: %v", err)
	}
	assertProfileStored(t, expected)
	storedUser, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil || !proto.Equal(storedUser, expectedUser) {
		t.Fatalf("profile mutations did not persist the expected user: %v", err)
	}
	userGet := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	if !proto.Equal(userGet.GetProfile(), expected) || userGet.GetUser().GetName() != expectedUser.GetName() ||
		userGet.GetUser().GetBuddyCardId() != expectedUser.GetBuddyCardId() || userGet.GetUser().GetEmblemId() != expectedUser.GetEmblemId() {
		t.Fatal("User.Get did not return the persisted profile/user settings")
	}
}

func TestProfilePhotoBackgroundScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewProfileClient(testConfig.GrpcConn)
	// A real master card is not selectable unless this user owns it.
	masters, err := opmaster.FindAllCards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, master := range masters {
		if slices.ContainsFunc(owned, func(card *ptransaction.UserCard) bool { return card.GetCardId() == master.GetId() }) {
			continue
		}
		profileReject(t, codes.NotFound, client.UpdateBuddyCard, &papi.ProfileUpdateBuddyCardRequest{CardId: master.GetId()})
		profileReject(t, codes.NotFound, client.UpdateBackground, &papi.ProfileUpdateBackgroundRequest{BackgroundType: penum.ProfileBackgroundType_ProfileBackgroundType_Card, FavoriteCardId: master.GetId()})
		break
	}
	levels, err := opmaster.FindAllPhotoRarityLevels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	photo := &ptransaction.UserPhoto{PhotoId: "profile-background-photo", AssetId: "profile-background-asset", ImageType: penum.PhotoImageType_PhotoImageType_Asset}
	var expectedReward int64
	for _, level := range levels {
		if level.GetDeleteRewardItem() > 0 {
			photo.Rarity, photo.Level = level.GetRarity(), level.GetLevel()
			expectedReward = int64(level.GetDeleteRewardItem())
			break
		}
	}
	if expectedReward == 0 {
		t.Fatal("no sellable photo rarity/level master found")
	}
	if err := opuser.UpsertUserPhoto(ctx, testConfig.TestUserSuid, photo); err != nil {
		t.Fatal(err)
	}
	expected, err := opuser.FindUserProfile(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	expected.BackgroundType = penum.ProfileBackgroundType_ProfileBackgroundType_Photo
	expected.FavoritePhotoId = photo.GetPhotoId()
	result := profileCall(t, client.UpdateBackground, &papi.ProfileUpdateBackgroundRequest{BackgroundType: expected.GetBackgroundType(), FavoritePhotoId: photo.GetPhotoId()})
	assertProfileDelta(t, result.GetCommonResponse(), expected)
	self := profileCall(t, client.FindUser, &papi.ProfileFindUserRequest{UserId: testConfig.TestUserPublicId})
	if self.GetFavoritePhoto().GetPhotoId() != photo.GetPhotoId() || self.GetFavoritePhoto().GetAssetId() != photo.GetAssetId() ||
		self.GetFavoritePhoto().GetImageType() != photo.GetImageType() || self.GetFavoriteCard() != nil {
		t.Fatalf("self profile did not project selected photo: %v", self)
	}
	photoClient := papi.NewPhotoClient(testConfig.GrpcConn)
	profileReject(t, codes.FailedPrecondition, photoClient.Delete, &papi.PhotoDeleteRequest{PhotoIds: []string{photo.GetPhotoId()}})
	stored, err := opuser.FindUserPhoto(ctx, testConfig.TestUserSuid, photo.GetPhotoId())
	if err != nil || !proto.Equal(stored, photo) {
		t.Fatalf("rejected sale changed the background photo: %v", err)
	}
	userGet := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	if !proto.Equal(userGet.GetProfile(), expected) {
		t.Fatal("User.Get lost the photo background")
	}
	expected.BackgroundType = penum.ProfileBackgroundType_ProfileBackgroundType_Default
	expected.FavoritePhotoId = ""
	cleared := profileCall(t, client.UpdateBackground, &papi.ProfileUpdateBackgroundRequest{BackgroundType: expected.GetBackgroundType()})
	assertProfileDelta(t, cleared.GetCommonResponse(), expected)
	self = profileCall(t, client.FindUser, &papi.ProfileFindUserRequest{UserId: testConfig.TestUserSuid})
	if self.GetFavoritePhoto() != nil || self.GetFavoriteCard() != nil {
		t.Fatal("default background retained stale card/photo info")
	}
	sale := profileCall(t, photoClient.Delete, &papi.PhotoDeleteRequest{PhotoIds: []string{photo.GetPhotoId()}})
	if len(sale.GetCommonResponse().GetDeletedData().GetPhotos()) != 1 {
		t.Fatal("photo was not sellable after removing it from the profile")
	}
	if _, err := opuser.FindUserPhoto(ctx, testConfig.TestUserSuid, photo.GetPhotoId()); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("sold photo still exists: %v", err)
	}
	assertProfileStored(t, expected)
}

func TestProfileBirthdayScenario(t *testing.T) {
	setupTestUser(t)
	client := papi.NewProfileClient(testConfig.GrpcConn)
	for _, birthday := range []*papi.ProfileUpdateBirthdayRequest{
		{}, {Month: 13, Day: 1}, {Month: 4, Day: 31}, {Month: 2, Day: 30}, {Month: 1, Day: -1},
	} {
		profileReject(t, codes.InvalidArgument, client.UpdateBirthday, birthday)
	}
	before, err := opuser.FindUserProfile(context.Background(), testConfig.TestUserSuid)
	if err != nil || before.GetBirthMonth() != 0 || before.GetBirthDay() != 0 {
		t.Fatalf("rejected dates changed the unset birthday: %v", err)
	}
	// Concurrent first-time submissions must still set a birthday only once.
	// A month/day birthday may validly be February 29, independently of this year.
	type birthdayResult struct {
		response *papi.ProfileUpdateBirthdayResponse
		err      error
	}
	results := make(chan birthdayResult, 2)
	for range 2 {
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cancel)
		go func() {
			response, err := client.UpdateBirthday(ctx, &papi.ProfileUpdateBirthdayRequest{Month: 2, Day: 29})
			results <- birthdayResult{response: response, err: err}
		}()
	}
	var response *papi.ProfileUpdateBirthdayResponse
	successes := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			successes++
			response = result.response
		} else if status.Code(result.err) != codes.FailedPrecondition {
			t.Fatalf("concurrent birthday submission returned %v", result.err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent birthday submissions succeeded %d times, want exactly once", successes)
	}
	before.BirthMonth, before.BirthDay = 2, 29
	assertProfileDelta(t, response.GetCommonResponse(), before)
	profileReject(t, codes.FailedPrecondition, client.UpdateBirthday, &papi.ProfileUpdateBirthdayRequest{Month: 12, Day: 31})
	assertProfileStored(t, before)
	userGet := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	if !proto.Equal(userGet.GetProfile(), before) {
		t.Fatal("User.Get did not retain the immutable birthday")
	}
	self := profileCall(t, client.FindUser, &papi.ProfileFindUserRequest{UserId: testConfig.TestUserPublicId})
	if self.GetBirthMonth() != 2 || self.GetBirthDay() != 29 {
		t.Fatal("self profile did not show the saved birthday")
	}
}

func profileCall[Req, Resp any](t *testing.T, call func(context.Context, Req, ...grpc.CallOption) (Resp, error), req Req) Resp {
	t.Helper()
	response, err := profileInvoke(t, call, req)
	if err != nil {
		t.Fatalf("profile scenario RPC failed: %v", err)
	}
	if common, ok := any(response).(interface{ GetCommonResponse() *papi.CommonResponse }); ok && common.GetCommonResponse() == nil {
		t.Fatal("profile scenario RPC omitted commonResponse")
	}
	return response
}

func profileReject[Req, Resp any](t *testing.T, want codes.Code, call func(context.Context, Req, ...grpc.CallOption) (Resp, error), req Req) {
	t.Helper()
	_, err := profileInvoke(t, call, req)
	if status.Code(err) != want {
		t.Fatalf("request %v returned %v, want %v", req, err, want)
	}
}

func profileInvoke[Req, Resp any](t *testing.T, call func(context.Context, Req, ...grpc.CallOption) (Resp, error), req Req) (Resp, error) {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	return call(ctx, req)
}

func assertProfileDelta(t *testing.T, common *papi.CommonResponse, expected *ptransaction.UserProfile) {
	t.Helper()
	if !proto.Equal(common.GetUpdatedData().GetProfile(), expected) {
		t.Fatalf("unexpected profile cache delta: got %v, want %v", common.GetUpdatedData().GetProfile(), expected)
	}
	assertProfileStored(t, expected)
}

func assertProfileStored(t *testing.T, expected *ptransaction.UserProfile) {
	t.Helper()
	stored, err := opuser.FindUserProfile(context.Background(), testConfig.TestUserSuid)
	if err != nil || !proto.Equal(stored, expected) {
		t.Fatalf("unexpected stored profile: got %v, want %v, error %v", stored, expected, err)
	}
}

func assertProfileHierarchyPreview(t *testing.T, expected *ptransaction.UserProfile, buddyID string) {
	t.Helper()
	top := profileCall(t, papi.NewHierarchyClient(testConfig.GrpcConn).Top, &papi.Empty{})
	buddy, err := opuser.FindUserCard(context.Background(), testConfig.TestUserSuid, buddyID)
	if err != nil {
		t.Fatal(err)
	}
	previews := 0
	for _, reward := range top.GetDivisionRewardInfos() {
		info := reward.GetFirstUserInfo()
		if info == nil {
			continue
		}
		previews++
		if info.GetUserId() != testConfig.TestUserPublicId || info.GetBackgroundType() != expected.GetBackgroundType() ||
			info.GetFavoriteCard().GetCardId() != expected.GetFavoriteCardId() ||
			info.GetFavoritePhoto().GetPhotoId() != expected.GetFavoritePhotoId() ||
			info.GetBuddyCardInfo().GetCardId() != buddyID || info.GetBuddyCardInfo().GetLevel() != buddy.GetLevel() ||
			info.GetBuddyCardInfo().GetRarity() != buddy.GetRarity() {
			t.Fatalf("Hierarchy.Top profile preview omitted current background/buddy: %v", info)
		}
	}
	if previews == 0 {
		t.Fatal("god user has no reached hierarchy division reward to verify its profile preview")
	}
}

func activateProfileMissions(t *testing.T) []string {
	t.Helper()
	masters, err := opmaster.FindAllMissions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 2)
	for _, wanted := range []penum.MissionType{penum.MissionType_MissionType_ProfileSetting, penum.MissionType_MissionType_PostProfileToTwitter} {
		for _, master := range masters {
			if master.GetMissionType() != wanted || len(master.GetTargetIds()) > 0 || len(master.GetProgresses()) == 0 {
				continue
			}
			if err := opuser.UpsertUserMission(context.Background(), testConfig.TestUserSuid, &ptransaction.UserMission{
				MissionId: master.GetId(), CurrentThreshold: master.GetProgresses()[0].GetThreshold(), StatusType: penum.MissionStatusType_MissionStatusType_Progress,
			}); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, master.GetId())
			break
		}
	}
	if len(ids) != 2 {
		t.Fatal("profile mission definitions are missing from master data")
	}
	return ids
}

func assertProfileMissionProgress(t *testing.T, ids []string) {
	t.Helper()
	missions, err := opuser.FindAllUserMissions(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		found := false
		for _, mission := range missions {
			if mission.GetMissionId() == id {
				found = true
				if mission.GetProgress() != 1 || mission.GetStatusType() == penum.MissionStatusType_MissionStatusType_Received {
					t.Fatalf("profile mission %s was not progressed exactly once without claiming: %v", id, mission)
				}
			}
		}
		if !found {
			t.Fatalf("profile mission %s disappeared", id)
		}
	}
}
