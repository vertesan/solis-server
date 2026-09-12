package test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

func TestPhotoScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	client := papi.NewPhotoClient(testConfig.GrpcConn)
	callCtx := func() (context.Context, context.CancelFunc) {
		t.Helper()
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		return ctx, cancel
	}

	listCtx, listCancel := callCtx()
	listResponse, err := client.ListShooting(listCtx, &papi.Empty{})
	listCancel()
	if err != nil {
		t.Fatalf("Photo.ListShooting failed: %v", err)
	}
	if listResponse.GetCommonResponse().GetUpdatedData() == nil || listResponse.GetCommonResponse().GetDeletedData() == nil {
		t.Fatal("Photo.ListShooting returned an incomplete common response")
	}
	assertPhotoShootingCatalogMatchesUserData(t, dbCtx, listResponse)

	music := firstUnlockedPhotoMusic(listResponse.GetMusics())
	stage := firstUnlockedPhotoStage(listResponse.GetStages())
	if music == nil || stage == nil {
		t.Fatal("fresh configured user has no unlocked music or stage for the photo scenario")
	}

	characters, costumes, hairs := photoQuestCast(t, dbCtx)
	checkRequest := &papi.PhotoCheckShootingRequest{
		ActionType:   penum.PhotoShootingActionType_PhotoShootingActionType_Quest,
		PhotoMusicId: music.GetMusicId(),
		PhotoStageId: stage.GetStageId(),
		CharacterIds: characters,
		CostumeIds:   costumes,
		HairIds:      hairs,
	}
	checkCtx, checkCancel := callCtx()
	checkResponse, err := client.CheckShooting(checkCtx, checkRequest)
	checkCancel()
	if err != nil {
		t.Fatalf("Photo.CheckShooting failed: %v", err)
	}
	if checkResponse.GetCommonResponse().GetUpdatedData() == nil || checkResponse.GetCommonResponse().GetDeletedData() == nil {
		t.Fatal("Photo.CheckShooting returned an incomplete common response")
	}

	userBefore, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	maxManagerLevel, err := opmaster.FindMaxManagerLevel(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	userBefore.ManagerLevel = maxManagerLevel.GetLevel()
	userBefore.ManagerExp = maxManagerLevel.GetRequiredExp()
	if err := opuser.UpsertUser(dbCtx, testConfig.TestUserSuid, userBefore); err != nil {
		t.Fatal(err)
	}
	if len(userBefore.GetNextPhotoImageIds()) == 0 {
		t.Fatal("configured test user has no reserved photo image IDs")
	}
	reservedImageIDs := slices.Clone(userBefore.GetNextPhotoImageIds())
	usedImageID := reservedImageIDs[0]

	const filmItemID = "item-photo_shooting-01"
	const filmSeedAmount int64 = 3
	if err := opuser.UpsertUserItem(dbCtx, testConfig.TestUserSuid, &ptransaction.UserItem{
		ItemId: filmItemID,
		Amount: filmSeedAmount,
	}); err != nil {
		t.Fatal(err)
	}
	focusCharacterBefore, err := opuser.FindUserCharacter(dbCtx, testConfig.TestUserSuid, characters[0])
	if err != nil {
		t.Fatal(err)
	}
	totalCountBefore, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}

	createRequest := &papi.PhotoCreateShootingsRequest{
		ActionType:   checkRequest.GetActionType(),
		PhotoMusicId: checkRequest.GetPhotoMusicId(),
		PhotoStageId: checkRequest.GetPhotoStageId(),
		CharacterIds: slices.Clone(checkRequest.GetCharacterIds()),
		CostumeIds:   slices.Clone(checkRequest.GetCostumeIds()),
		HairIds:      slices.Clone(checkRequest.GetHairIds()),
		CreateShootingParams: []*papi.PhotoCreateShootingParam{{
			PhotoImageId:    usedImageID,
			ItemId:          filmItemID,
			MainCharacterId: characters[0],
		}},
		// Captured official traffic shows that this is not the number of photos.
		ManualCount: 7,
	}
	createCtx, createCancel := callCtx()
	createResponse, err := client.CreateShootings(createCtx, createRequest)
	createCancel()
	if err != nil {
		t.Fatalf("Photo.CreateShootings failed: %v", err)
	}
	if createResponse.GetIsGift() {
		t.Fatal("the first photo of a fresh user was unexpectedly delivered as a gift")
	}
	if len(createResponse.GetPhotos()) != 1 {
		t.Fatalf("Photo.CreateShootings returned %d photos for one create parameter", len(createResponse.GetPhotos()))
	}
	created := createResponse.GetPhotos()[0]
	if created.GetPhotoId() == "" || created.GetAssetId() != usedImageID ||
		created.GetImageType() != penum.PhotoImageType_PhotoImageType_User ||
		created.GetFocusCharacterId() != characters[0] {
		t.Fatalf("Photo.CreateShootings returned an invalid photo identity: %v", created)
	}
	assertGeneratedPhotoIsMasterValid(t, dbCtx, created)
	createdUpdated := photoWithID(createResponse.GetCommonResponse().GetUpdatedData().GetPhotos(), created.GetPhotoId())
	if createdUpdated == nil || !proto.Equal(createdUpdated, created) {
		t.Fatal("Photo.CreateShootings did not include the created photo in updatedData")
	}
	if createResponse.GetCommonResponse().GetUpdatedData().GetUser() == nil ||
		createResponse.GetCommonResponse().GetUpdatedData().GetTotalCount() == nil {
		t.Fatal("Photo.CreateShootings omitted the updated user or total count")
	}

	stored, err := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, created.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(stored, created) {
		t.Fatalf("stored photo differs from CreateShootings response: stored=%v response=%v", stored, created)
	}
	if amount := photoPermanentItemAmount(t, dbCtx, filmItemID); amount != filmSeedAmount-1 {
		t.Fatalf("shooting film amount = %d, want %d", amount, filmSeedAmount-1)
	}
	focusCharacterAfter, err := opuser.FindUserCharacter(dbCtx, testConfig.TestUserSuid, characters[0])
	if err != nil {
		t.Fatal(err)
	}
	if focusCharacterAfter.GetReliability() != focusCharacterBefore.GetReliability()+1 {
		t.Fatalf("focus character reliability = %d, want %d", focusCharacterAfter.GetReliability(), focusCharacterBefore.GetReliability()+1)
	}
	totalCountAfter, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if totalCountAfter.GetPhotoShootCount() != totalCountBefore.GetPhotoShootCount()+1 {
		t.Fatalf("photo shoot count = %d, want %d", totalCountAfter.GetPhotoShootCount(), totalCountBefore.GetPhotoShootCount()+1)
	}
	userAfterCreate, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(userAfterCreate.GetNextPhotoImageIds()) != len(reservedImageIDs) ||
		slices.Contains(userAfterCreate.GetNextPhotoImageIds(), usedImageID) ||
		!allPhotoImageIDsUnique(userAfterCreate.GetNextPhotoImageIds()) {
		t.Fatalf("CreateShootings did not rotate reserved image IDs correctly: before=%v after=%v", reservedImageIDs, userAfterCreate.GetNextPhotoImageIds())
	}

	userGetCtx, userGetCancel := callCtx()
	userGetResponse, err := papi.NewUserClient(testConfig.GrpcConn).Get(userGetCtx, &papi.Empty{})
	userGetCancel()
	if err != nil {
		t.Fatalf("User.Get after shooting failed: %v", err)
	}
	if photoWithID(userGetResponse.GetPhotos(), created.GetPhotoId()) == nil {
		t.Fatal("User.Get omitted the newly created photo")
	}
	if userGetResponse.GetTotalCount().GetPhotoShootCount() != totalCountAfter.GetPhotoShootCount() ||
		!slices.Equal(userGetResponse.GetUser().GetNextPhotoImageIds(), userAfterCreate.GetNextPhotoImageIds()) {
		t.Fatal("User.Get did not expose the persisted shooting state")
	}

	const renamedPhoto = "integration test photo"
	updateCtx, updateCancel := callCtx()
	updateResponse, err := client.Update(updateCtx, &papi.PhotoUpdateRequest{
		PhotoId: created.GetPhotoId(),
		Name:    "  " + renamedPhoto + "  ",
	})
	updateCancel()
	if err != nil {
		t.Fatalf("Photo.Update failed: %v", err)
	}
	renamed := photoWithID(updateResponse.GetCommonResponse().GetUpdatedData().GetPhotos(), created.GetPhotoId())
	if renamed == nil || renamed.GetName() != renamedPhoto {
		t.Fatalf("Photo.Update returned an invalid post-image: %v", renamed)
	}
	stored, err = opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, created.GetPhotoId())
	if err != nil || stored.GetName() != renamedPhoto {
		t.Fatalf("Photo.Update did not persist the trimmed name: photo=%v err=%v", stored, err)
	}

	lockCtx, lockCancel := callCtx()
	lockResponse, err := client.SwitchLock(lockCtx, &papi.PhotoSwitchLockRequest{PhotoIds: []string{created.GetPhotoId()}})
	lockCancel()
	if err != nil {
		t.Fatalf("Photo.SwitchLock (lock) failed: %v", err)
	}
	locked := photoWithID(lockResponse.GetCommonResponse().GetUpdatedData().GetPhotos(), created.GetPhotoId())
	if locked == nil || !locked.GetLocked() {
		t.Fatal("Photo.SwitchLock did not return the locked post-image")
	}
	stored, err = opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, created.GetPhotoId())
	if err != nil || !stored.GetLocked() {
		t.Fatalf("Photo.SwitchLock did not persist locked state: photo=%v err=%v", stored, err)
	}
	lockedDeleteCtx, lockedDeleteCancel := callCtx()
	_, err = client.Delete(lockedDeleteCtx, &papi.PhotoDeleteRequest{PhotoIds: []string{created.GetPhotoId()}})
	lockedDeleteCancel()
	if err == nil {
		t.Fatal("Photo.Delete accepted a locked photo")
	}
	if stored, findErr := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, created.GetPhotoId()); findErr != nil || !stored.GetLocked() {
		t.Fatalf("rejected locked-photo deletion changed the photo: photo=%v err=%v", stored, findErr)
	}

	unlockCtx, unlockCancel := callCtx()
	unlockResponse, err := client.SwitchLock(unlockCtx, &papi.PhotoSwitchLockRequest{PhotoIds: []string{created.GetPhotoId()}})
	unlockCancel()
	if err != nil {
		t.Fatalf("Photo.SwitchLock (unlock) failed: %v", err)
	}
	unlocked := photoWithID(unlockResponse.GetCommonResponse().GetUpdatedData().GetPhotos(), created.GetPhotoId())
	if unlocked == nil || unlocked.GetLocked() {
		t.Fatal("Photo.SwitchLock did not return the unlocked post-image")
	}

	// A normal shooting is rerollable. Keep the mutation scenario deterministic
	// if a future random dedicated-photo branch produces a non-rerollable result.
	if !unlocked.GetRerollable() {
		unlocked.Rerollable = true
		if err := opuser.UpsertUserPhoto(dbCtx, testConfig.TestUserSuid, unlocked); err != nil {
			t.Fatal(err)
		}
	}
	beforeReroll := proto.Clone(unlocked).(*ptransaction.UserPhoto)
	rarityLevel, err := opmaster.FindPhotoRarityLevel(dbCtx, beforeReroll.GetRarity(), beforeReroll.GetLevel())
	if err != nil {
		t.Fatal(err)
	}
	rerollSeedAmount := int64(rarityLevel.GetRerollRequiredItem()) + 17
	if err := opuser.UpsertUserItem(dbCtx, testConfig.TestUserSuid, &ptransaction.UserItem{
		ItemId: "item-photo_reroll",
		Amount: rerollSeedAmount,
	}); err != nil {
		t.Fatal(err)
	}
	rerollCtx, rerollCancel := callCtx()
	rerollResponse, err := client.Reroll(rerollCtx, &papi.PhotoRerollRequest{PhotoId: created.GetPhotoId()})
	rerollCancel()
	if err != nil {
		t.Fatalf("Photo.Reroll failed: %v", err)
	}
	rerolled := photoWithID(rerollResponse.GetCommonResponse().GetUpdatedData().GetPhotos(), created.GetPhotoId())
	if rerolled == nil {
		t.Fatal("Photo.Reroll omitted the updated photo")
	}
	assertPhotoIdentityExceptRandomOptions(t, beforeReroll, rerolled)
	assertRerolledPhotoOptionsAreMasterValid(t, dbCtx, rerolled)
	if amount := photoPermanentItemAmount(t, dbCtx, "item-photo_reroll"); amount != rerollSeedAmount-int64(rarityLevel.GetRerollRequiredItem()) {
		t.Fatalf("reroll item amount = %d, want %d", amount, rerollSeedAmount-int64(rarityLevel.GetRerollRequiredItem()))
	}

	rerolled.IsDreamBase = true
	if err := opuser.UpsertUserPhoto(dbCtx, testConfig.TestUserSuid, rerolled); err != nil {
		t.Fatal(err)
	}
	rerollAmountBeforeRejectedDream := photoPermanentItemAmount(t, dbCtx, "item-photo_reroll")
	dreamRerollCtx, dreamRerollCancel := callCtx()
	_, err = client.Reroll(dreamRerollCtx, &papi.PhotoRerollRequest{PhotoId: created.GetPhotoId()})
	dreamRerollCancel()
	if err == nil {
		t.Fatal("Photo.Reroll accepted an image-training base photo")
	}
	stored, findErr := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, created.GetPhotoId())
	if findErr != nil || !stored.GetIsDreamBase() ||
		photoPermanentItemAmount(t, dbCtx, "item-photo_reroll") != rerollAmountBeforeRejectedDream {
		t.Fatalf("rejected image-training reroll changed user data: photo=%v err=%v", stored, findErr)
	}
	rerolled.IsDreamBase = false
	if err := opuser.UpsertUserPhoto(dbCtx, testConfig.TestUserSuid, rerolled); err != nil {
		t.Fatal(err)
	}

	rarityLevels, err := opmaster.FindPhotoRarityLevels(dbCtx, rerolled.GetRarity())
	if err != nil {
		t.Fatal(err)
	}
	var enhanceTargetLevel int32
	var enhanceCost int64
	for _, level := range rarityLevels {
		if level.GetLevel() <= rerolled.GetLevel() || level.GetLevel() > maxManagerLevel.GetMaxPhotoEnhanceLevel() {
			continue
		}
		enhanceTargetLevel = level.GetLevel()
		enhanceCost = int64(level.GetEnhanceRequiredItem())
		break
	}
	if enhanceTargetLevel == 0 {
		t.Fatalf("no master-derived enhance target exists above generated level %d", rerolled.GetLevel())
	}
	enhanceSeedAmount := enhanceCost + 19
	if err := opuser.UpsertUserItem(dbCtx, testConfig.TestUserSuid, &ptransaction.UserItem{
		ItemId: "item-photo_enhance",
		Amount: enhanceSeedAmount,
	}); err != nil {
		t.Fatal(err)
	}
	enhanceCtx, enhanceCancel := callCtx()
	enhanceResponse, err := client.Enhance(enhanceCtx, &papi.PhotoEnhanceRequest{
		PhotoId: created.GetPhotoId(),
		Level:   enhanceTargetLevel,
	})
	enhanceCancel()
	if err != nil {
		t.Fatalf("Photo.Enhance failed: %v", err)
	}
	enhanced := photoWithID(enhanceResponse.GetCommonResponse().GetUpdatedData().GetPhotos(), created.GetPhotoId())
	if enhanced == nil || enhanced.GetLevel() != enhanceTargetLevel {
		t.Fatalf("Photo.Enhance returned level %d, want %d", enhanced.GetLevel(), enhanceTargetLevel)
	}
	stored, err = opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, created.GetPhotoId())
	if err != nil || stored.GetLevel() != enhanceTargetLevel {
		t.Fatalf("Photo.Enhance did not persist target level: photo=%v err=%v", stored, err)
	}
	if amount := photoPermanentItemAmount(t, dbCtx, "item-photo_enhance"); amount != enhanceSeedAmount-enhanceCost {
		t.Fatalf("photo enhance item amount = %d, want %d", amount, enhanceSeedAmount-enhanceCost)
	}

	deleteLevel, err := opmaster.FindPhotoRarityLevel(dbCtx, enhanced.GetRarity(), enhanced.GetLevel())
	if err != nil {
		t.Fatal(err)
	}
	rerollAmountBeforeDelete := photoPermanentItemAmount(t, dbCtx, "item-photo_reroll")
	profile, err := opuser.FindUserProfile(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	originalFavoritePhotoID := profile.GetFavoritePhotoId()
	profile.FavoritePhotoId = created.GetPhotoId()
	if err := opuser.UpsertUserProfile(dbCtx, testConfig.TestUserSuid, profile); err != nil {
		t.Fatal(err)
	}
	favoriteDeleteCtx, favoriteDeleteCancel := callCtx()
	_, err = client.Delete(favoriteDeleteCtx, &papi.PhotoDeleteRequest{PhotoIds: []string{created.GetPhotoId()}})
	favoriteDeleteCancel()
	if err == nil {
		t.Fatal("Photo.Delete accepted the business-card photo")
	}
	if _, findErr := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, created.GetPhotoId()); findErr != nil ||
		photoPermanentItemAmount(t, dbCtx, "item-photo_reroll") != rerollAmountBeforeDelete {
		t.Fatalf("rejected business-card deletion changed user data: %v", findErr)
	}
	profile.FavoritePhotoId = originalFavoritePhotoID
	if err := opuser.UpsertUserProfile(dbCtx, testConfig.TestUserSuid, profile); err != nil {
		t.Fatal(err)
	}
	deleteCtx, deleteCancel := callCtx()
	deleteResponse, err := client.Delete(deleteCtx, &papi.PhotoDeleteRequest{PhotoIds: []string{created.GetPhotoId()}})
	deleteCancel()
	if err != nil {
		t.Fatalf("Photo.Delete failed: %v", err)
	}
	if len(deleteResponse.GetCommonResponse().GetDeletedData().GetPhotos()) != 1 ||
		!proto.Equal(deleteResponse.GetCommonResponse().GetDeletedData().GetPhotos()[0], &ptransaction.UserPhoto{PhotoId: created.GetPhotoId()}) {
		t.Fatalf("Photo.Delete returned invalid deletedData: %v", deleteResponse.GetCommonResponse().GetDeletedData().GetPhotos())
	}
	if _, err := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, created.GetPhotoId()); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("deleted photo is still present: %v", err)
	}
	wantRerollAmount := rerollAmountBeforeDelete + int64(deleteLevel.GetDeleteRewardItem())
	if amount := photoPermanentItemAmount(t, dbCtx, "item-photo_reroll"); amount != wantRerollAmount {
		t.Fatalf("photo deletion reward amount = %d, want %d", amount, wantRerollAmount)
	}

	setting, err := opmaster.FindSetting(dbCtx, 1)
	if err != nil {
		t.Fatal(err)
	}
	userBeforeExtend, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if userBeforeExtend.GetPhotoLimitExtendedCount() >= setting.GetPhotoLimitExtendLimit() {
		t.Fatalf("fresh test user's photo extension count %d is already at the master limit", userBeforeExtend.GetPhotoLimitExtendedCount())
	}
	balanceSeed := &ptransaction.UserBalance{
		FreeBalance: setting.GetPhotoLimitExtendStone() + 23,
		PaidBalance: 11,
	}
	if err := opuser.UpsertUserBalance(dbCtx, testConfig.TestUserSuid, balanceSeed); err != nil {
		t.Fatal(err)
	}
	extendCtx, extendCancel := callCtx()
	extendResponse, err := client.ExtendLimit(extendCtx, &papi.Empty{})
	extendCancel()
	if err != nil {
		t.Fatalf("Photo.ExtendLimit failed: %v", err)
	}
	updatedUser := extendResponse.GetCommonResponse().GetUpdatedData().GetUser()
	updatedBalance := extendResponse.GetCommonResponse().GetUpdatedData().GetBalance()
	if updatedUser.GetPhotoLimitExtendedCount() != userBeforeExtend.GetPhotoLimitExtendedCount()+1 {
		t.Fatalf("photo extension count = %d, want %d", updatedUser.GetPhotoLimitExtendedCount(), userBeforeExtend.GetPhotoLimitExtendedCount()+1)
	}
	if updatedBalance.GetFreeBalance() != balanceSeed.GetFreeBalance()-setting.GetPhotoLimitExtendStone() ||
		updatedBalance.GetPaidBalance() != balanceSeed.GetPaidBalance() {
		t.Fatalf("Photo.ExtendLimit returned invalid balance: %v", updatedBalance)
	}

	finalGetCtx, finalGetCancel := callCtx()
	finalGet, err := papi.NewUserClient(testConfig.GrpcConn).Get(finalGetCtx, &papi.Empty{})
	finalGetCancel()
	if err != nil {
		t.Fatalf("final User.Get failed: %v", err)
	}
	if photoWithID(finalGet.GetPhotos(), created.GetPhotoId()) != nil {
		t.Fatal("final User.Get still contains the deleted photo")
	}
	if finalGet.GetUser().GetPhotoLimitExtendedCount() != updatedUser.GetPhotoLimitExtendedCount() ||
		!proto.Equal(finalGet.GetBalance(), updatedBalance) {
		t.Fatal("final User.Get does not match the persisted limit extension")
	}
}

func TestPhotoOverflowGiftScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	fixture := preparePhotoVariantShooting(t, dbCtx)
	photoClient := papi.NewPhotoClient(testConfig.GrpcConn)
	giftClient := papi.NewGiftClient(testConfig.GrpcConn)

	listCtx, listCancel := photoGRPCCallContext(t)
	list, err := photoClient.ListShooting(listCtx, &papi.Empty{})
	listCancel()
	if err != nil {
		t.Fatalf("Photo.ListShooting failed: %v", err)
	}
	music := firstUnlockedPhotoMusic(list.GetMusics())
	stage := firstUnlockedPhotoStage(list.GetStages())
	if music == nil || stage == nil {
		t.Fatal("configured test user has no unlocked photo quest music or stage")
	}
	characterIDs, costumeIDs, hairIDs := photoQuestCast(t, dbCtx)

	settingID, err := config.GetGameSettingId()
	if err != nil {
		t.Fatal(err)
	}
	setting, err := opmaster.FindSetting(dbCtx, settingID)
	if err != nil {
		t.Fatal(err)
	}
	user, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	capacity := int64(setting.GetPhotoDefaultLimit()) +
		int64(user.GetPhotoLimitExtendedCount())*int64(setting.GetPhotoLimitExtendAmount())
	ownedBefore, err := opuser.FindAllUserPhotos(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if capacity < int64(len(ownedBefore))+1 {
		t.Fatalf("master-derived photo capacity %d cannot fit the configured user's %d photos", capacity, len(ownedBefore))
	}

	rarityLevels, err := opmaster.FindAllPhotoRarityLevels(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	var fillerLevel *pmaster.PhotoRarityLevel
	for _, level := range rarityLevels {
		if level.GetRarity() > 0 && level.GetLevel() > 0 && level.GetDeleteRewardItem() > 0 {
			fillerLevel = level
			break
		}
	}
	if fillerLevel == nil {
		t.Fatal("master data has no deletable photo rarity level for the overflow scenario")
	}
	fillerCount := capacity - int64(len(ownedBefore)) - 1
	if fillerCount < 2 {
		t.Fatalf("photo capacity fixture needs at least two removable photos, got %d", fillerCount)
	}
	fillers := make([]*ptransaction.UserPhoto, 0, int(fillerCount))
	for index := int64(0); index < fillerCount; index++ {
		fillers = append(fillers, &ptransaction.UserPhoto{
			PhotoId:      fmt.Sprintf("photo-overflow-filler-%06d", index),
			Name:         "overflow capacity fixture",
			Rarity:       fillerLevel.GetRarity(),
			Level:        fillerLevel.GetLevel(),
			ShootingTime: index + 1,
		})
	}
	if err := opuser.UpsertManyUserPhotos(dbCtx, testConfig.TestUserSuid, fillers); err != nil {
		t.Fatal(err)
	}
	ownedAtStart, err := opuser.FindAllUserPhotos(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(ownedAtStart)) != capacity-1 {
		t.Fatalf("overflow fixture owns %d photos, want %d", len(ownedAtStart), capacity-1)
	}

	user, err = opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(user.GetNextPhotoImageIds()) < 3 {
		t.Fatal("configured test user has fewer than three reserved photo image IDs")
	}
	createParams := make([]*papi.PhotoCreateShootingParam, 3)
	focusCounts := make(map[string]int32, len(createParams))
	reliabilityBefore := make(map[string]int32, len(createParams))
	for index := range createParams {
		characterID := characterIDs[index]
		createParams[index] = &papi.PhotoCreateShootingParam{
			PhotoImageId:    user.GetNextPhotoImageIds()[index],
			ItemId:          fixture.filmID,
			MainCharacterId: characterID,
		}
		focusCounts[characterID]++
		character, err := opuser.FindUserCharacter(dbCtx, testConfig.TestUserSuid, characterID)
		if err != nil {
			t.Fatal(err)
		}
		reliabilityBefore[characterID] = character.GetReliability()
	}
	createCtx, createCancel := photoGRPCCallContext(t)
	created, err := photoClient.CreateShootings(createCtx, &papi.PhotoCreateShootingsRequest{
		ActionType:           penum.PhotoShootingActionType_PhotoShootingActionType_Quest,
		PhotoMusicId:         music.GetMusicId(),
		PhotoStageId:         stage.GetStageId(),
		CharacterIds:         characterIDs,
		CostumeIds:           costumeIDs,
		HairIds:              hairIDs,
		CreateShootingParams: createParams,
		ManualCount:          0,
	})
	createCancel()
	if err != nil {
		t.Fatalf("Photo.CreateShootings overflow flow failed: %v", err)
	}
	if !created.GetIsGift() || len(created.GetPhotos()) != 3 ||
		len(created.GetCommonResponse().GetUpdatedData().GetPhotos()) != 1 {
		t.Fatalf("Photo.CreateShootings returned an invalid overflow split: %v", created)
	}
	wantFilmAmount := photoVariantFilmSeedAmount - int64(len(createParams))
	if amount := photoPermanentItemAmount(t, dbCtx, fixture.filmID); amount != wantFilmAmount {
		t.Fatalf("overflow shooting film amount = %d, want %d", amount, wantFilmAmount)
	}
	if item := photoItemWithID(created.GetCommonResponse().GetUpdatedData().GetItems(), fixture.filmID); item == nil ||
		item.GetExpiredTime() != 0 || item.GetAmount() != wantFilmAmount {
		t.Fatalf("overflow shooting omitted the film post-image: %v", item)
	}
	totalAfterCreate, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if totalAfterCreate.GetPhotoShootCount() != fixture.photoShootCountBefore+int64(len(createParams)) ||
		created.GetCommonResponse().GetUpdatedData().GetTotalCount().GetPhotoShootCount() != totalAfterCreate.GetPhotoShootCount() {
		t.Fatalf("overflow photos were omitted from the shooting count: stored=%d response=%v",
			totalAfterCreate.GetPhotoShootCount(), created.GetCommonResponse().GetUpdatedData().GetTotalCount())
	}
	for characterID, count := range focusCounts {
		wantReliability := reliabilityBefore[characterID] + count
		updatedCharacter := photoCharacterWithID(
			created.GetCommonResponse().GetUpdatedData().GetCharacters(), characterID,
		)
		if updatedCharacter == nil || updatedCharacter.GetReliability() != wantReliability {
			t.Fatalf("overflow shooting omitted reliability for %q: got %v, want %d", characterID, updatedCharacter, wantReliability)
		}
		storedCharacter, err := opuser.FindUserCharacter(dbCtx, testConfig.TestUserSuid, characterID)
		if err != nil || storedCharacter.GetReliability() != wantReliability {
			t.Fatalf("overflow shooting did not persist reliability for %q: got %v err=%v", characterID, storedCharacter, err)
		}
	}
	usedImageIDs := make(map[string]struct{}, len(createParams))
	for _, param := range createParams {
		usedImageIDs[param.GetPhotoImageId()] = struct{}{}
	}
	assertRotatedImageIDs := func(label string, ids []string) {
		t.Helper()
		if len(ids) != len(user.GetNextPhotoImageIds()) {
			t.Fatalf("%s reserved %d image IDs, want %d", label, len(ids), len(user.GetNextPhotoImageIds()))
		}
		seen := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			if id == "" {
				t.Fatalf("%s contains an empty reserved image ID", label)
			}
			if _, used := usedImageIDs[id]; used {
				t.Fatalf("%s retained used image ID %q", label, id)
			}
			if _, duplicate := seen[id]; duplicate {
				t.Fatalf("%s contains duplicate image ID %q", label, id)
			}
			seen[id] = struct{}{}
		}
	}
	assertRotatedImageIDs("Photo.CreateShootings response", created.GetCommonResponse().GetUpdatedData().GetUser().GetNextPhotoImageIds())
	storedUser, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	assertRotatedImageIDs("stored user", storedUser.GetNextPhotoImageIds())
	if _, err := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, created.GetPhotos()[0].GetPhotoId()); err != nil {
		t.Fatalf("capacity-fitting photo was not persisted: %v", err)
	}

	overflowPhotos := created.GetPhotos()[1:]
	stagedByID := make(map[string]*ptransaction.UserPhoto, len(overflowPhotos))
	for _, overflow := range overflowPhotos {
		for _, ability := range overflow.GetAbilities() {
			if ability.GetIsAvailable() {
				t.Fatalf("overflow response exposed an available ability: %v", overflow)
			}
		}
		if _, err := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, overflow.GetPhotoId()); !errors.Is(err, mongo.ErrNoDocuments) {
			t.Fatalf("overflow photo unexpectedly entered owned photos: %v", err)
		}
		staged, err := opuser.FindUserPhotoGift(dbCtx, testConfig.TestUserSuid, overflow.GetPhotoId())
		if err != nil {
			t.Fatalf("overflow photo was not staged: %v", err)
		}
		for _, ability := range staged.GetAbilities() {
			if !ability.GetIsAvailable() {
				t.Fatalf("staged overflow photo persisted a disabled ability: %v", staged)
			}
		}
		responseForm := proto.Clone(staged).(*ptransaction.UserPhoto)
		for _, ability := range responseForm.GetAbilities() {
			ability.IsAvailable = false
		}
		if !proto.Equal(responseForm, overflow) {
			t.Fatalf("staged photo identity differs from the overflow response: staged=%v response=%v", staged, overflow)
		}
		stagedByID[staged.GetPhotoId()] = staged
	}
	if notification := photoGiftNotification(created.GetCommonResponse().GetUpdatedData().GetNotifications()); notification == nil || !notification.GetValid() {
		t.Fatalf("overflow shooting did not enable the gift notification: %v", notification)
	}

	listGiftCtx, listGiftCancel := photoGRPCCallContext(t)
	giftList, err := giftClient.List(listGiftCtx, &papi.GiftListRequest{})
	listGiftCancel()
	if err != nil {
		t.Fatalf("Gift.List after overflow failed: %v", err)
	}
	giftIDs := make([]string, 0, len(overflowPhotos))
	const giftLifetimeMillis = int64((14 * 24 * time.Hour) / time.Millisecond)
	for _, overflow := range overflowPhotos {
		gift := photoGiftForResource(giftList.GetGifts(), overflow.GetPhotoId())
		if gift == nil || gift.GetResourceType() != penum.ResourceType_ResourceType_PhotoGift ||
			gift.GetAmount() != 1 || gift.GetPhotoGift().GetName() != overflow.GetName() ||
			gift.GetMessage() != "所持上限を超えたためギフトに送られました" ||
			gift.GetLimitTime()-gift.GetPostedTime() != giftLifetimeMillis {
			t.Fatalf("Gift.List returned an invalid overflow gift for %q: %v", overflow.GetPhotoId(), gift)
		}
		giftIDs = append(giftIDs, gift.GetGiftId())
	}

	fullReceiveCtx, fullReceiveCancel := photoGRPCCallContext(t)
	fullReceive, err := giftClient.Receive(fullReceiveCtx, &papi.GiftReceiveRequest{GiftIds: giftIDs})
	fullReceiveCancel()
	if err != nil {
		t.Fatalf("Gift.Receive at full photo capacity failed: %v", err)
	}
	if len(fullReceive.GetResults()) != 0 || len(fullReceive.GetReceivedGiftIds()) != 0 ||
		!slices.Equal(fullReceive.GetUnreceivedGiftIds(), giftIDs) || !fullReceive.GetHasUnreceivedGift() ||
		!proto.Equal(fullReceive.GetCommonResponse().GetUpdatedData(), &papi.UpdatedData{}) {
		t.Fatalf("Gift.Receive at full capacity was not a successful no-op: %v", fullReceive)
	}

	for _, filler := range fillers[:2] {
		deleteCtx, deleteCancel := photoGRPCCallContext(t)
		_, err = photoClient.Delete(deleteCtx, &papi.PhotoDeleteRequest{PhotoIds: []string{filler.GetPhotoId()}})
		deleteCancel()
		if err != nil {
			t.Fatalf("Photo.Delete while opening gift capacity failed: %v", err)
		}
	}

	receiveCtx, receiveCancel := photoGRPCCallContext(t)
	received, err := giftClient.Receive(receiveCtx, &papi.GiftReceiveRequest{GiftIds: giftIDs})
	receiveCancel()
	if err != nil {
		t.Fatalf("Gift.Receive for staged photos failed: %v", err)
	}
	if !slices.Equal(received.GetReceivedGiftIds(), giftIDs) || len(received.GetUnreceivedGiftIds()) != 0 ||
		received.GetHasUnreceivedGift() || len(received.GetResults()) != len(giftIDs) {
		t.Fatalf("Gift.Receive returned an invalid staged-photo result: %v", received)
	}
	for index, result := range received.GetResults() {
		wantBefore := capacity - 2 + int64(index)
		if result.GetResourceType() != penum.ResourceType_ResourceType_PhotoGift ||
			result.GetResourceId() != overflowPhotos[index].GetPhotoId() ||
			result.GetBeforeAmount() != wantBefore || result.GetAfterAmount() != wantBefore+1 ||
			result.GetAmountI64() != 1 || !result.GetIsNew() {
			t.Fatalf("received photo result %d is invalid: %v", index, result)
		}
	}
	for photoID, staged := range stagedByID {
		receivedPhoto := photoWithID(received.GetCommonResponse().GetUpdatedData().GetPhotos(), photoID)
		if !proto.Equal(receivedPhoto, staged) {
			t.Fatalf("Gift.Receive did not restore staged photo %q", photoID)
		}
		for _, ability := range receivedPhoto.GetAbilities() {
			if !ability.GetIsAvailable() {
				t.Fatalf("Gift.Receive returned a disabled staged-photo ability: %v", receivedPhoto)
			}
		}
		stored, err := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, photoID)
		if err != nil || !proto.Equal(stored, staged) {
			t.Fatalf("received photo %q was not persisted: stored=%v err=%v", photoID, stored, err)
		}
		if _, err := opuser.FindUserPhotoGift(dbCtx, testConfig.TestUserSuid, photoID); !errors.Is(err, mongo.ErrNoDocuments) {
			t.Fatalf("received photo %q is still staged: %v", photoID, err)
		}
	}
	for _, giftID := range giftIDs {
		if _, err := opuser.FindUserGift(dbCtx, testConfig.TestUserSuid, giftID); !errors.Is(err, mongo.ErrNoDocuments) {
			t.Fatalf("received photo gift %q still exists: %v", giftID, err)
		}
	}
	if notification := photoGiftNotification(received.GetCommonResponse().GetUpdatedData().GetNotifications()); notification == nil || notification.GetValid() {
		t.Fatalf("receiving the last overflow gift did not clear its notification: %v", notification)
	}
	histories, err := opuser.FindAllUserGiftHistories(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for index, giftID := range giftIDs {
		var history *ptransaction.UserGiftHistory
		for _, candidate := range histories {
			if candidate.GetGiftId() == giftID {
				history = candidate
				break
			}
		}
		if history == nil || history.GetResourceId() != overflowPhotos[index].GetPhotoId() ||
			history.GetPhotoGift().GetName() != overflowPhotos[index].GetName() {
			t.Fatalf("received photo gift history %q is invalid: %v", giftID, history)
		}
	}
	ownedAfter, err := opuser.FindAllUserPhotos(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(ownedAfter)) != capacity {
		t.Fatalf("owned photo count after receiving gifts = %d, want %d", len(ownedAfter), capacity)
	}
}

func TestPhotoImageURLScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	user, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(user.GetNextPhotoImageIds()) == 0 {
		t.Fatal("configured test user has no reserved photo image IDs")
	}
	public, err := opuser.FindUserPublic(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}

	assetID := user.GetNextPhotoImageIds()[0]
	objectName := "Photo/" + public.GetPublicUserId() + "/" + assetID
	now := time.Now().UTC()
	expiresAt := now.Add(time.Hour)
	const uploadID = "photo-image-url-upload"
	const finalizationID = "photo-image-url-finalize"
	const downloadToken = "photo-image-url-download-token"
	metadata := &umodel.UserPhotoImageMetadata{
		ID:                   objectName,
		SUID:                 testConfig.TestUserSuid,
		PublicUserId:         public.GetPublicUserId(),
		UploadId:             uploadID,
		UploadState:          umodel.UserPhotoImageUploadActive,
		UploadStateUpdatedAt: now,
		UploadExpiresAt:      &expiresAt,
		Name:                 objectName,
		Bucket:               "udc-idolypride",
		ContentType:          "image/jpeg",
		DownloadTokens:       downloadToken,
	}
	started, err := opuser.StartUserPhotoImageUpload(dbCtx, metadata, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !started {
		t.Fatal("failed to create photo image upload metadata fixture")
	}
	claimed, err := opuser.ClaimUserPhotoImageUpload(
		dbCtx, objectName, testConfig.TestUserSuid, uploadID, finalizationID, now, expiresAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("failed to claim photo image upload metadata fixture")
	}
	metadata.UploadState = umodel.UserPhotoImageUploadFinalized
	metadata.FinalizationId = finalizationID
	metadata.UploadStateUpdatedAt = now
	metadata.UploadExpiresAt = nil
	metadata.Generation = strconv.FormatInt(now.UnixMicro(), 10)
	completed, err := opuser.CompleteUserPhotoImageUpload(dbCtx, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("failed to finalize photo image upload metadata fixture")
	}

	ctx, cancel := photoGRPCCallContext(t)
	response, err := papi.NewPhotoClient(testConfig.GrpcConn).GetImageURL(ctx, &papi.PhotoGetImageUrlRequest{
		Type:     penum.PhotoImageRequestType_PhotoImageRequestType_Both,
		AssetIds: []string{assetID},
	})
	cancel()
	if err != nil {
		t.Fatalf("Photo.GetImageURL failed: %v", err)
	}
	if len(response.GetUrlList()) != 1 {
		t.Fatalf("Photo.GetImageURL returned %d entries, want 1", len(response.GetUrlList()))
	}
	imageURL := response.GetUrlList()[0]
	if imageURL.GetAssetId() != assetID || imageURL.GetOrigUrl() == "" || imageURL.GetOrigUrl() != imageURL.GetTmbUrl() {
		t.Fatalf("Photo.GetImageURL returned an invalid both-image entry: %v", imageURL)
	}
	parsed, err := url.Parse(imageURL.GetOrigUrl())
	if err != nil {
		t.Fatalf("Photo.GetImageURL returned an invalid URL: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "firebasestorage.googleapis.com" ||
		parsed.EscapedPath() != "/v0/b/udc-idolypride/o/"+url.PathEscape(objectName) ||
		parsed.Query().Get("alt") != "media" || parsed.Query().Get("token") != downloadToken || parsed.Query().Get("v") != metadata.Generation {
		t.Fatalf("Photo.GetImageURL returned an unexpected Firebase-compatible URL: %s", imageURL.GetOrigUrl())
	}

	// The published URL remains usable while a replacement generation uploads.
	metadata.UploadId = "photo-image-url-replacement"
	metadata.UploadState = umodel.UserPhotoImageUploadActive
	metadata.UploadExpiresAt = &expiresAt
	started, err = opuser.StartUserPhotoImageUpload(dbCtx, metadata, now.Add(-time.Hour))
	if err != nil || !started {
		t.Fatalf("failed to start image URL replacement fixture: started=%v err=%v", started, err)
	}
	ctx, cancel = photoGRPCCallContext(t)
	replacementResponse, err := papi.NewPhotoClient(testConfig.GrpcConn).GetImageURL(ctx, &papi.PhotoGetImageUrlRequest{
		Type: penum.PhotoImageRequestType_PhotoImageRequestType_Both, AssetIds: []string{assetID},
	})
	cancel()
	if err != nil || len(replacementResponse.GetUrlList()) != 1 || !proto.Equal(replacementResponse.GetUrlList()[0], imageURL) {
		t.Fatalf("pending replacement lost the published image URL: response=%v err=%v", replacementResponse, err)
	}
}

func TestPhotoRetouchScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	fixture := selectPhotoRetouchFixture(t, dbCtx)
	photo := &ptransaction.UserPhoto{
		PhotoId:      "photo-retouch-scenario",
		AssetId:      "photo-retouch-scenario-image",
		ImageType:    penum.PhotoImageType_PhotoImageType_User,
		CharacterIds: []string{fixture.focusCharacterID},
		Name:         "retouch scenario",
		Rarity:       1,
		Level:        30,
		Rerollable:   false,
		Abilities: []*ptransaction.UserPhotoAbility{{
			PhotoAbilityId: "source-photo-option",
			EffectValue:    1,
			Grade:          1,
			IsAvailable:    true,
		}},
		ShootingTime:     time.Now().UnixMilli(),
		FocusCharacterId: fixture.focusCharacterID,
	}
	if err := opuser.UpsertUserPhoto(dbCtx, testConfig.TestUserSuid, photo); err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserPhotoRecipe(dbCtx, testConfig.TestUserSuid, &ptransaction.UserPhotoRecipe{
		PhotoRecipeId: fixture.recipe.GetId(),
		Amount:        2,
	}); err != nil {
		t.Fatal(err)
	}

	itemCosts := photoConsumptionItemCosts(t, fixture.consumption)
	itemAmountsBefore := make(map[string]int64, len(itemCosts))
	for itemID, cost := range itemCosts {
		amount := cost + 41
		itemAmountsBefore[itemID] = amount
		if err := opuser.UpsertUserItem(dbCtx, testConfig.TestUserSuid, &ptransaction.UserItem{
			ItemId: itemID,
			Amount: amount,
		}); err != nil {
			t.Fatal(err)
		}
	}
	totalBefore, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}

	dreamBase := proto.Clone(photo).(*ptransaction.UserPhoto)
	dreamBase.IsDreamBase = true
	if err := opuser.UpsertUserPhoto(dbCtx, testConfig.TestUserSuid, dreamBase); err != nil {
		t.Fatal(err)
	}
	rejectCtx, rejectCancel := photoGRPCCallContext(t)
	_, err = papi.NewPhotoClient(testConfig.GrpcConn).Retouch(rejectCtx, &papi.PhotoRetouchRequest{
		PhotoRecipeId: fixture.recipe.GetId(),
		PhotoId:       photo.GetPhotoId(),
	})
	rejectCancel()
	if err == nil {
		t.Fatal("Photo.Retouch accepted an image-training base photo")
	}
	storedRejected, findErr := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, photo.GetPhotoId())
	if findErr != nil || !proto.Equal(storedRejected, dreamBase) {
		t.Fatalf("rejected retouch changed the photo: photo=%v err=%v", storedRejected, findErr)
	}
	for itemID, amount := range itemAmountsBefore {
		if got := photoPermanentItemAmount(t, dbCtx, itemID); got != amount {
			t.Fatalf("rejected retouch changed resource %q: got %d, want %d", itemID, got, amount)
		}
	}
	storedRecipe, err := opuser.FindUserPhotoRecipe(dbCtx, testConfig.TestUserSuid, fixture.recipe.GetId())
	if err != nil || storedRecipe.GetAmount() != 2 {
		t.Fatalf("rejected retouch changed recipe state: recipe=%v err=%v", storedRecipe, err)
	}
	if err := opuser.UpsertUserPhoto(dbCtx, testConfig.TestUserSuid, photo); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := photoGRPCCallContext(t)
	response, err := papi.NewPhotoClient(testConfig.GrpcConn).Retouch(ctx, &papi.PhotoRetouchRequest{
		PhotoRecipeId: fixture.recipe.GetId(),
		PhotoId:       photo.GetPhotoId(),
	})
	cancel()
	if err != nil {
		t.Fatalf("Photo.Retouch failed: %v", err)
	}
	updated := response.GetCommonResponse().GetUpdatedData()
	if updated == nil || response.GetCommonResponse().GetDeletedData() == nil {
		t.Fatal("Photo.Retouch returned an incomplete common response")
	}
	retouched := photoWithID(updated.GetPhotos(), photo.GetPhotoId())
	if retouched == nil {
		t.Fatal("Photo.Retouch omitted the updated photo")
	}
	if retouched.GetPhotoId() != photo.GetPhotoId() || retouched.GetAssetId() != photo.GetAssetId() ||
		retouched.GetImageType() != photo.GetImageType() || retouched.GetName() != photo.GetName() ||
		retouched.GetPlaceName() != photo.GetPlaceName() || retouched.GetEventName() != photo.GetEventName() ||
		retouched.GetLocked() != photo.GetLocked() || retouched.GetShootingTime() != photo.GetShootingTime() ||
		retouched.GetFocusCharacterId() != photo.GetFocusCharacterId() {
		t.Fatalf("Photo.Retouch changed photo identity fields: before=%v after=%v", photo, retouched)
	}
	if retouched.GetRarity() != fixture.abilitySet.GetPhotoRarity() ||
		!slices.Contains(fixture.resultLevels, retouched.GetLevel()) ||
		!slices.Equal(retouched.GetCharacterIds(), fixture.recipe.GetCharacterIds()) ||
		retouched.GetPhotoRecipeId() != fixture.recipe.GetId() || retouched.GetRerollable() ||
		retouched.GetIsDreamBase() || retouched.GetDreamMaterialPhotoRarity() != 0 ||
		retouched.GetDreamMaterialPhotoLevel() != 0 || retouched.GetDreamMaterialPhotoRecipeId() != "" {
		t.Fatalf("Photo.Retouch did not replace source traits with recipe traits: %v", retouched)
	}
	assertTestPhotoAbilitiesMatchMaster(
		t, retouched.GetAbilities(), fixture.abilitySet, fixture.abilities, retouched.GetLevel(),
	)

	updatedRecipe := photoRecipeWithID(updated.GetPhotoRecipes(), fixture.recipe.GetId())
	if updatedRecipe == nil || updatedRecipe.GetAmount() != 1 {
		t.Fatalf("Photo.Retouch returned an invalid recipe post-image: %v", updatedRecipe)
	}
	for itemID, cost := range itemCosts {
		wantAmount := itemAmountsBefore[itemID] - cost
		if amount := photoPermanentItemAmount(t, dbCtx, itemID); amount != wantAmount {
			t.Fatalf("retouch resource %q amount = %d, want %d", itemID, amount, wantAmount)
		}
		item := photoItemWithID(updated.GetItems(), itemID)
		if item == nil || item.GetExpiredTime() != 0 || item.GetAmount() != wantAmount {
			t.Fatalf("Photo.Retouch returned an invalid resource post-image for %q: %v", itemID, item)
		}
	}
	if updated.GetTotalCount().GetPhotoRetouchCount() != totalBefore.GetPhotoRetouchCount()+1 {
		t.Fatalf("photo retouch count = %d, want %d", updated.GetTotalCount().GetPhotoRetouchCount(), totalBefore.GetPhotoRetouchCount()+1)
	}

	storedPhoto, err := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, photo.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	storedRecipe, err = opuser.FindUserPhotoRecipe(dbCtx, testConfig.TestUserSuid, fixture.recipe.GetId())
	if err != nil {
		t.Fatal(err)
	}
	storedTotal, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(storedPhoto, retouched) || !proto.Equal(storedRecipe, updatedRecipe) ||
		storedTotal.GetPhotoRetouchCount() != updated.GetTotalCount().GetPhotoRetouchCount() {
		t.Fatal("Photo.Retouch response does not match persisted state")
	}

	getCtx, getCancel := photoGRPCCallContext(t)
	getResponse, err := papi.NewUserClient(testConfig.GrpcConn).Get(getCtx, &papi.Empty{})
	getCancel()
	if err != nil {
		t.Fatalf("User.Get after retouch failed: %v", err)
	}
	if !proto.Equal(photoWithID(getResponse.GetPhotos(), photo.GetPhotoId()), retouched) ||
		!proto.Equal(photoRecipeWithID(getResponse.GetPhotoRecipes(), fixture.recipe.GetId()), updatedRecipe) ||
		getResponse.GetTotalCount().GetPhotoRetouchCount() != updated.GetTotalCount().GetPhotoRetouchCount() {
		t.Fatal("User.Get did not expose the persisted retouch state")
	}
}

func TestPhotoRecipeSaleScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	recipe, rewardResource := selectPhotoRecipeSaleFixture(t, dbCtx)
	settingID, err := config.GetGameSettingId()
	if err != nil {
		t.Fatal(err)
	}
	setting, err := opmaster.FindSetting(dbCtx, settingID)
	if err != nil {
		t.Fatal(err)
	}
	saleCount := int64(2)
	if setting.GetPhotoRecipeSaleAmountLimit() < int32(saleCount) {
		saleCount = int64(setting.GetPhotoRecipeSaleAmountLimit())
	}
	if saleCount <= 0 {
		t.Fatal("master data does not allow selling any photo recipes")
	}

	if err := opuser.UpsertUserPhotoRecipe(dbCtx, testConfig.TestUserSuid, &ptransaction.UserPhotoRecipe{
		PhotoRecipeId: recipe.GetId(),
		Amount:        saleCount,
	}); err != nil {
		t.Fatal(err)
	}
	const rewardItemBefore int64 = 37
	if err := opuser.UpsertUserItem(dbCtx, testConfig.TestUserSuid, &ptransaction.UserItem{
		ItemId: rewardResource.GetResourceId(),
		Amount: rewardItemBefore,
	}); err != nil {
		t.Fatal(err)
	}
	requestIDs := make([]string, saleCount)
	for index := range requestIDs {
		requestIDs[index] = recipe.GetId()
	}

	ctx, cancel := photoGRPCCallContext(t)
	response, err := papi.NewPhotoClient(testConfig.GrpcConn).SaleRecipe(ctx, &papi.PhotoSaleRecipeRequest{
		PhotoRecipeIds: requestIDs,
	})
	cancel()
	if err != nil {
		t.Fatalf("Photo.SaleRecipe failed: %v", err)
	}
	wantRewardAmount := saleCount * int64(rewardResource.GetAmount())
	wantItemAmount := rewardItemBefore + wantRewardAmount
	updated := response.GetCommonResponse().GetUpdatedData()
	updatedRecipe := photoRecipeWithID(updated.GetPhotoRecipes(), recipe.GetId())
	if updatedRecipe == nil || updatedRecipe.GetAmount() != 0 {
		t.Fatalf("Photo.SaleRecipe returned an invalid recipe post-image: %v", updatedRecipe)
	}
	updatedItem := photoItemWithID(updated.GetItems(), rewardResource.GetResourceId())
	if updatedItem == nil || updatedItem.GetExpiredTime() != 0 || updatedItem.GetAmount() != wantItemAmount {
		t.Fatalf("Photo.SaleRecipe returned an invalid reward item post-image: %v", updatedItem)
	}
	if len(response.GetRewardResults()) != 1 {
		t.Fatalf("Photo.SaleRecipe returned %d reward results, want 1 aggregated result", len(response.GetRewardResults()))
	}
	result := response.GetRewardResults()[0]
	if result.GetResourceType() != penum.ResourceType_ResourceType_Item ||
		result.GetResourceId() != rewardResource.GetResourceId() ||
		result.GetBeforeAmount() != rewardItemBefore || result.GetAfterAmount() != wantItemAmount ||
		result.GetAmountI64() != wantRewardAmount {
		t.Fatalf("Photo.SaleRecipe returned an invalid reward result: %v", result)
	}
	if amount := photoPermanentItemAmount(t, dbCtx, rewardResource.GetResourceId()); amount != wantItemAmount {
		t.Fatalf("persisted sale reward amount = %d, want %d", amount, wantItemAmount)
	}
	storedRecipe, err := opuser.FindUserPhotoRecipe(dbCtx, testConfig.TestUserSuid, recipe.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(storedRecipe, updatedRecipe) {
		t.Fatal("Photo.SaleRecipe recipe response does not match persisted state")
	}

	getCtx, getCancel := photoGRPCCallContext(t)
	getResponse, err := papi.NewUserClient(testConfig.GrpcConn).Get(getCtx, &papi.Empty{})
	getCancel()
	if err != nil {
		t.Fatalf("User.Get after recipe sale failed: %v", err)
	}
	if !proto.Equal(photoRecipeWithID(getResponse.GetPhotoRecipes(), recipe.GetId()), updatedRecipe) ||
		photoItemWithID(getResponse.GetItems(), rewardResource.GetResourceId()).GetAmount() != wantItemAmount {
		t.Fatal("User.Get did not expose the persisted recipe sale state")
	}
}

func TestPhotoRecipeCapacityScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	recipe, _ := selectPhotoRecipeSaleFixture(t, dbCtx)
	settingID, err := config.GetGameSettingId()
	if err != nil {
		t.Fatal(err)
	}
	setting, err := opmaster.FindSetting(dbCtx, settingID)
	if err != nil {
		t.Fatal(err)
	}
	if setting.GetPhotoRecipeLimit() <= 0 {
		t.Fatal("master data has no positive photo recipe capacity")
	}
	masters, err := opmaster.FindAllPhotoRecipes(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	var capacityRecipe *pmaster.PhotoRecipe
	for _, candidate := range masters {
		if candidate.GetId() != "" && candidate.GetId() != recipe.GetId() && candidate.GetSaleRewardId() != "" {
			capacityRecipe = candidate
			break
		}
	}
	if capacityRecipe == nil {
		t.Fatal("master data has no second saleable photo recipe for the capacity scenario")
	}
	existing, err := opuser.FindAllUserPhotoRecipes(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range existing {
		value.Amount = 0
		if err := opuser.UpsertUserPhotoRecipe(dbCtx, testConfig.TestUserSuid, value); err != nil {
			t.Fatal(err)
		}
	}
	targetAmount := setting.GetPhotoRecipeLimit() / 2
	if targetAmount <= 0 {
		t.Fatal("photo recipe capacity is too small for a multi-recipe scenario")
	}
	capacityAmount := setting.GetPhotoRecipeLimit() - targetAmount
	if err := opuser.UpsertUserPhotoRecipe(dbCtx, testConfig.TestUserSuid, &ptransaction.UserPhotoRecipe{
		PhotoRecipeId: recipe.GetId(),
		Amount:        targetAmount,
	}); err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserPhotoRecipe(dbCtx, testConfig.TestUserSuid, &ptransaction.UserPhotoRecipe{
		PhotoRecipeId: capacityRecipe.GetId(),
		Amount:        capacityAmount,
	}); err != nil {
		t.Fatal(err)
	}

	const giftID = "photo-recipe-capacity-gift"
	if err := opuser.UpsertUserGift(dbCtx, testConfig.TestUserSuid, &ptransaction.UserGift{
		GiftId:       giftID,
		ResourceType: penum.ResourceType_ResourceType_PhotoRecipe,
		ResourceId:   recipe.GetId(),
		Amount:       1,
		PostedTime:   time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserNotification(dbCtx, testConfig.TestUserSuid, &ptransaction.UserNotification{
		NotificationType: penum.NotificationType_NotificationType_Gift,
		Valid:            true,
	}); err != nil {
		t.Fatal(err)
	}

	giftClient := papi.NewGiftClient(testConfig.GrpcConn)
	listCtx, listCancel := photoGRPCCallContext(t)
	listed, err := giftClient.List(listCtx, &papi.GiftListRequest{
		Filters: []penum.GiftFilterType{penum.GiftFilterType_GiftFilterType_Photo},
	})
	listCancel()
	if err != nil {
		t.Fatalf("Gift.List for a photo recipe failed: %v", err)
	}
	listedRecipeGift := false
	for _, gift := range listed.GetGifts() {
		if gift.GetGiftId() == giftID && gift.GetResourceType() == penum.ResourceType_ResourceType_PhotoRecipe {
			listedRecipeGift = true
			break
		}
	}
	if !listedRecipeGift {
		t.Fatal("the photo gift filter omitted a photo recipe gift")
	}

	fullCtx, fullCancel := photoGRPCCallContext(t)
	fullResponse, err := giftClient.Receive(fullCtx, &papi.GiftReceiveRequest{GiftIds: []string{giftID}})
	fullCancel()
	if err != nil {
		t.Fatalf("Gift.Receive at full recipe capacity failed: %v", err)
	}
	if len(fullResponse.GetReceivedGiftIds()) != 0 ||
		!slices.Equal(fullResponse.GetUnreceivedGiftIds(), []string{giftID}) ||
		len(fullResponse.GetResults()) != 0 || !fullResponse.GetHasUnreceivedGift() ||
		!proto.Equal(fullResponse.GetCommonResponse().GetUpdatedData(), &papi.UpdatedData{}) {
		t.Fatalf("Gift.Receive at full recipe capacity was not a successful no-op: %v", fullResponse)
	}
	if _, err := opuser.FindUserGift(dbCtx, testConfig.TestUserSuid, giftID); err != nil {
		t.Fatalf("full-capacity receive removed the recipe gift: %v", err)
	}
	notification, err := opuser.FindUserNotification(
		dbCtx, testConfig.TestUserSuid, penum.NotificationType_NotificationType_Gift,
	)
	if err != nil || !notification.GetValid() {
		t.Fatalf("full-capacity receive cleared the gift notification: notification=%v err=%v", notification, err)
	}
	storedRecipe, err := opuser.FindUserPhotoRecipe(dbCtx, testConfig.TestUserSuid, recipe.GetId())
	if err != nil || storedRecipe.GetAmount() != targetAmount {
		t.Fatalf("full-capacity receive changed recipe state: recipe=%v err=%v", storedRecipe, err)
	}

	saleCtx, saleCancel := photoGRPCCallContext(t)
	saleResponse, err := papi.NewPhotoClient(testConfig.GrpcConn).SaleRecipe(saleCtx, &papi.PhotoSaleRecipeRequest{
		PhotoRecipeIds: []string{capacityRecipe.GetId()},
	})
	saleCancel()
	if err != nil {
		t.Fatalf("Photo.SaleRecipe while opening recipe capacity failed: %v", err)
	}
	if sold := photoRecipeWithID(saleResponse.GetCommonResponse().GetUpdatedData().GetPhotoRecipes(), capacityRecipe.GetId()); sold == nil || sold.GetAmount() != capacityAmount-1 {
		t.Fatalf("Photo.SaleRecipe did not open one recipe slot: %v", sold)
	}

	receiveCtx, receiveCancel := photoGRPCCallContext(t)
	received, err := giftClient.Receive(receiveCtx, &papi.GiftReceiveRequest{GiftIds: []string{giftID}})
	receiveCancel()
	if err != nil {
		t.Fatalf("Gift.Receive after opening recipe capacity failed: %v", err)
	}
	if !slices.Equal(received.GetReceivedGiftIds(), []string{giftID}) ||
		len(received.GetUnreceivedGiftIds()) != 0 || len(received.GetResults()) != 1 ||
		received.GetHasUnreceivedGift() {
		t.Fatalf("Gift.Receive returned an invalid recipe result: %v", received)
	}
	result := received.GetResults()[0]
	if result.GetResourceType() != penum.ResourceType_ResourceType_PhotoRecipe ||
		result.GetResourceId() != recipe.GetId() || result.GetAmountI64() != 1 ||
		result.GetBeforeAmount() != targetAmount || result.GetAfterAmount() != targetAmount+1 {
		t.Fatalf("Gift.Receive returned an inconsistent recipe amount: %v", result)
	}
	storedRecipe, err = opuser.FindUserPhotoRecipe(dbCtx, testConfig.TestUserSuid, recipe.GetId())
	if err != nil || storedRecipe.GetAmount() != targetAmount+1 {
		t.Fatalf("received recipe was not persisted at the capacity: recipe=%v err=%v", storedRecipe, err)
	}
	storedCapacityRecipe, err := opuser.FindUserPhotoRecipe(dbCtx, testConfig.TestUserSuid, capacityRecipe.GetId())
	if err != nil || storedCapacityRecipe.GetAmount()+storedRecipe.GetAmount() != setting.GetPhotoRecipeLimit() {
		t.Fatalf("received recipe did not restore the total-copy capacity: capacityRecipe=%v targetRecipe=%v err=%v",
			storedCapacityRecipe, storedRecipe, err)
	}
	if updatedRecipe := photoRecipeWithID(received.GetCommonResponse().GetUpdatedData().GetPhotoRecipes(), recipe.GetId()); !proto.Equal(updatedRecipe, storedRecipe) {
		t.Fatalf("Gift.Receive omitted the recipe post-image: updated=%v stored=%v", updatedRecipe, storedRecipe)
	}
	if _, err := opuser.FindUserGift(dbCtx, testConfig.TestUserSuid, giftID); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("received recipe gift still exists: %v", err)
	}
	if notification := photoGiftNotification(received.GetCommonResponse().GetUpdatedData().GetNotifications()); notification == nil || notification.GetValid() {
		t.Fatalf("receiving the recipe gift did not clear its notification: %v", notification)
	}
	storedNotification, err := opuser.FindUserNotification(
		dbCtx, testConfig.TestUserSuid, penum.NotificationType_NotificationType_Gift,
	)
	if err != nil || storedNotification.GetValid() {
		t.Fatalf("receiving the recipe gift did not persist the cleared notification: notification=%v err=%v", storedNotification, err)
	}
	histories, err := opuser.FindAllUserGiftHistories(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	var history *ptransaction.UserGiftHistory
	for _, candidate := range histories {
		if candidate.GetGiftId() == giftID {
			history = candidate
			break
		}
	}
	if history == nil || history.GetResourceType() != penum.ResourceType_ResourceType_PhotoRecipe ||
		history.GetResourceId() != recipe.GetId() || history.GetAmount() != 1 {
		t.Fatalf("received recipe gift history is invalid: %v", history)
	}
}

func TestDreamBasePhotoEnhanceLevelInfoScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	fixture := selectDreamPhotoFixture(t, dbCtx)
	characters, err := opuser.FindAllUserCharacters(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(characters) == 0 {
		t.Fatal("configured test user has no characters for the dream photo fixture")
	}
	photo := &ptransaction.UserPhoto{
		PhotoId:                  "photo-dream-level-info-scenario",
		AssetId:                  "photo-dream-level-info-image",
		ImageType:                penum.PhotoImageType_PhotoImageType_Generated,
		CharacterIds:             []string{characters[0].GetCharacterId()},
		Name:                     "dream level info scenario",
		Rarity:                   fixture.abilitySet.GetPhotoRarity(),
		Level:                    fixture.level,
		Abilities:                buildTestPhotoAbilities(t, fixture.abilitySet, fixture.abilities, fixture.level),
		ShootingTime:             time.Now().UnixMilli(),
		FocusCharacterId:         characters[0].GetCharacterId(),
		IsDreamBase:              true,
		DreamMaterialPhotoRarity: fixture.abilitySet.GetPhotoRarity(),
		DreamMaterialPhotoLevel:  fixture.level,
	}
	if err := opuser.UpsertUserPhoto(dbCtx, testConfig.TestUserSuid, photo); err != nil {
		t.Fatal(err)
	}
	storedBefore, err := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, photo.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := photoGRPCCallContext(t)
	response, err := papi.NewPhotoClient(testConfig.GrpcConn).GetDreamBasePhotoEnhanceLevelInfos(
		ctx,
		&papi.GetDreamBasePhotoEnhanceLevelInfosRequest{BasePhotoId: photo.GetPhotoId()},
	)
	cancel()
	if err != nil {
		t.Fatalf("Photo.GetDreamBasePhotoEnhanceLevelInfos failed: %v", err)
	}
	if len(response.GetLevelInfos()) != 2 {
		t.Fatalf("Photo.GetDreamBasePhotoEnhanceLevelInfos returned %d levels, want current and next", len(response.GetLevelInfos()))
	}
	for index, level := range []int32{fixture.level, fixture.level + 5} {
		info := response.GetLevelInfos()[index]
		if info.GetLevel() != level || len(info.GetEffectValueInfos()) != len(photo.GetAbilities()) {
			t.Fatalf("invalid dream enhance level info at index %d: %v", index, info)
		}
		for abilityIndex, userAbility := range photo.GetAbilities() {
			effect := info.GetEffectValueInfos()[abilityIndex]
			wantEffect, ok := testPhotoAbilityEffectValue(fixture.abilities[userAbility.GetPhotoAbilityId()], level, userAbility.GetGrade())
			if !ok || effect.GetPhotoAbilityId() != userAbility.GetPhotoAbilityId() || effect.GetEffectValue() != wantEffect {
				t.Fatalf("invalid dream effect at level %d, index %d: got=%v wantAbility=%q wantEffect=%d", level, abilityIndex, effect, userAbility.GetPhotoAbilityId(), wantEffect)
			}
		}
	}
	if !proto.Equal(response.GetCommonResponse().GetUpdatedData(), &papi.UpdatedData{}) ||
		!proto.Equal(response.GetCommonResponse().GetDeletedData(), &papi.DeletedData{}) {
		t.Fatalf("read-only dream level query returned mutations: %v", response.GetCommonResponse())
	}
	storedAfter, err := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, photo.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(storedBefore, storedAfter) {
		t.Fatal("Photo.GetDreamBasePhotoEnhanceLevelInfos mutated the stored photo")
	}

	getCtx, getCancel := photoGRPCCallContext(t)
	getResponse, err := papi.NewUserClient(testConfig.GrpcConn).Get(getCtx, &papi.Empty{})
	getCancel()
	if err != nil {
		t.Fatalf("User.Get after dream level query failed: %v", err)
	}
	if !proto.Equal(photoWithID(getResponse.GetPhotos(), photo.GetPhotoId()), storedBefore) {
		t.Fatal("User.Get did not return the unchanged dream base photo")
	}
}

func TestPhotoRetouchedDreamBaseEnhanceScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	fixture := selectRetouchedDreamBaseEnhanceFixture(t, dbCtx)
	user, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	managerLevel, err := opmaster.FindMaxManagerLevel(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.targetLevel > managerLevel.GetMaxPhotoEnhanceLevel() {
		t.Fatalf("fixture target level %d exceeds maximum manager photo level %d", fixture.targetLevel, managerLevel.GetMaxPhotoEnhanceLevel())
	}
	user.ManagerLevel = managerLevel.GetLevel()
	user.ManagerExp = managerLevel.GetRequiredExp()
	if err := opuser.UpsertUser(dbCtx, testConfig.TestUserSuid, user); err != nil {
		t.Fatal(err)
	}

	characters, err := opuser.FindAllUserCharacters(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(characters) == 0 {
		t.Fatal("configured test user has no character for the enhancement fixture")
	}
	photo := &ptransaction.UserPhoto{
		PhotoId:                    "photo-retouched-dream-enhance-scenario",
		AssetId:                    "photo-retouched-dream-enhance-image",
		ImageType:                  penum.PhotoImageType_PhotoImageType_Generated,
		Name:                       "retouched dream enhance scenario",
		Rarity:                     fixture.abilitySet.GetPhotoRarity(),
		Level:                      fixture.currentLevel,
		Abilities:                  buildTestPhotoAbilities(t, fixture.abilitySet, fixture.abilities, fixture.currentLevel),
		FocusCharacterId:           characters[0].GetCharacterId(),
		CharacterIds:               append([]string(nil), fixture.recipe.GetCharacterIds()...),
		PhotoRecipeId:              fixture.recipe.GetId(),
		IsDreamBase:                true,
		DreamMaterialPhotoRarity:   fixture.abilitySet.GetPhotoRarity(),
		DreamMaterialPhotoLevel:    fixture.currentLevel,
		DreamMaterialPhotoRecipeId: "",
	}
	if err := opuser.UpsertUserPhoto(dbCtx, testConfig.TestUserSuid, photo); err != nil {
		t.Fatal(err)
	}

	costs := photoConsumptionItemCosts(t, fixture.retouchConsumption)
	costs["item-photo_enhance"] += int64(fixture.materialTarget.GetEnhanceRequiredItem())
	shortItemID := ""
	for itemID := range costs {
		if itemID != "item-photo_enhance" {
			shortItemID = itemID
			break
		}
	}
	if shortItemID == "" {
		t.Fatal("retouched enhancement fixture has no retouch-ink component")
	}
	amountsBeforeRejected := make(map[string]int64, len(costs))
	for itemID, cost := range costs {
		amount := cost + 23
		if itemID == shortItemID {
			amount = cost - 1
		}
		amountsBeforeRejected[itemID] = amount
		if err := opuser.UpsertUserItem(dbCtx, testConfig.TestUserSuid, &ptransaction.UserItem{
			ItemId: itemID,
			Amount: amount,
		}); err != nil {
			t.Fatal(err)
		}
	}

	client := papi.NewPhotoClient(testConfig.GrpcConn)
	rejectCtx, rejectCancel := photoGRPCCallContext(t)
	_, err = client.Enhance(rejectCtx, &papi.PhotoEnhanceRequest{
		PhotoId: photo.GetPhotoId(),
		Level:   fixture.targetLevel,
	})
	rejectCancel()
	if err == nil {
		t.Fatal("Photo.Enhance accepted insufficient retouch components")
	}
	storedRejected, findErr := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, photo.GetPhotoId())
	if findErr != nil || !proto.Equal(storedRejected, photo) {
		t.Fatalf("rejected retouched DreamBase enhancement changed the photo: photo=%v err=%v", storedRejected, findErr)
	}
	for itemID, amount := range amountsBeforeRejected {
		if got := photoPermanentItemAmount(t, dbCtx, itemID); got != amount {
			t.Fatalf("rejected enhancement changed item %q: got %d, want %d", itemID, got, amount)
		}
	}

	amountsBeforeSuccess := maps.Clone(amountsBeforeRejected)
	amountsBeforeSuccess[shortItemID] = costs[shortItemID] + 23
	if err := opuser.UpsertUserItem(dbCtx, testConfig.TestUserSuid, &ptransaction.UserItem{
		ItemId: shortItemID,
		Amount: amountsBeforeSuccess[shortItemID],
	}); err != nil {
		t.Fatal(err)
	}
	enhanceCtx, enhanceCancel := photoGRPCCallContext(t)
	response, err := client.Enhance(enhanceCtx, &papi.PhotoEnhanceRequest{
		PhotoId: photo.GetPhotoId(),
		Level:   fixture.targetLevel,
	})
	enhanceCancel()
	if err != nil {
		t.Fatalf("Photo.Enhance for a retouched DreamBase photo failed: %v", err)
	}
	updated := response.GetCommonResponse().GetUpdatedData()
	enhanced := photoWithID(updated.GetPhotos(), photo.GetPhotoId())
	if enhanced == nil || enhanced.GetLevel() != fixture.targetLevel ||
		enhanced.GetDreamMaterialPhotoLevel() != fixture.targetLevel ||
		enhanced.GetPhotoRecipeId() != fixture.recipe.GetId() || !enhanced.GetIsDreamBase() ||
		enhanced.GetDreamMaterialPhotoRarity() != photo.GetDreamMaterialPhotoRarity() ||
		enhanced.GetDreamMaterialPhotoRecipeId() != photo.GetDreamMaterialPhotoRecipeId() {
		t.Fatalf("Photo.Enhance returned invalid retouched DreamBase state: %v", enhanced)
	}
	if len(enhanced.GetAbilities()) != len(photo.GetAbilities()) {
		t.Fatalf("Photo.Enhance changed the option count: before=%v after=%v", photo.GetAbilities(), enhanced.GetAbilities())
	}
	for index, before := range photo.GetAbilities() {
		after := enhanced.GetAbilities()[index]
		wantEffect, ok := testPhotoAbilityEffectValue(
			fixture.abilities[before.GetPhotoAbilityId()], fixture.targetLevel, before.GetGrade(),
		)
		if !ok || after.GetPhotoAbilityId() != before.GetPhotoAbilityId() ||
			after.GetGrade() != before.GetGrade() || after.GetEffectValue() != wantEffect ||
			after.GetMissionId() != before.GetMissionId() || after.GetIsAvailable() != before.GetIsAvailable() {
			t.Fatalf("Photo.Enhance changed an option instead of its effect: before=%v after=%v", before, after)
		}
	}
	for itemID, cost := range costs {
		wantAmount := amountsBeforeSuccess[itemID] - cost
		if got := photoPermanentItemAmount(t, dbCtx, itemID); got != wantAmount {
			t.Fatalf("enhancement item %q amount = %d, want %d", itemID, got, wantAmount)
		}
		if item := photoItemWithID(updated.GetItems(), itemID); item == nil || item.GetAmount() != wantAmount {
			t.Fatalf("Photo.Enhance omitted item post-image for %q: %v", itemID, item)
		}
	}
	storedEnhanced, err := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, photo.GetPhotoId())
	if err != nil || !proto.Equal(storedEnhanced, enhanced) {
		t.Fatalf("Photo.Enhance response differs from stored photo: stored=%v response=%v err=%v", storedEnhanced, enhanced, err)
	}
}

func TestPhotoStoryEpisodeShootingScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	fixture := preparePhotoVariantShooting(t, dbCtx)
	episode := selectPhotoStoryEpisodeFixture(t, dbCtx)
	if err := opuser.UpsertUserStory(dbCtx, testConfig.TestUserSuid, &ptransaction.UserStory{
		StoryId:    episode.storyID,
		StatusType: penum.StoryStatusType_StoryStatusType_Read,
	}); err != nil {
		t.Fatal(err)
	}

	client := papi.NewPhotoClient(testConfig.GrpcConn)
	checkCtx, checkCancel := photoGRPCCallContext(t)
	checkResponse, err := client.CheckStoryEpisodeShooting(checkCtx, &papi.PhotoCheckStoryEpisodeShootingRequest{
		StoryPartId: episode.part.GetId(),
		Chapter:     episode.chapter,
		Route:       episode.route,
		Episode:     episode.episode,
	})
	checkCancel()
	if err != nil {
		t.Fatalf("Photo.CheckStoryEpisodeShooting failed: %v", err)
	}
	assertPhotoCheckResponseIsReadOnly(t, checkResponse.GetCommonResponse())

	createCtx, createCancel := photoGRPCCallContext(t)
	response, err := client.CreateStoryEpisodeShootings(createCtx, &papi.PhotoCreateStoryEpisodeShootingsRequest{
		StoryPartId: episode.part.GetId(),
		Chapter:     episode.chapter,
		Route:       episode.route,
		Episode:     episode.episode,
		CreateShootingParams: []*papi.PhotoCreateShootingParam{{
			PhotoImageId:    fixture.imageID,
			ItemId:          fixture.filmID,
			MainCharacterId: fixture.characterID,
		}},
		ManualCount: 2,
	})
	createCancel()
	if err != nil {
		t.Fatalf("Photo.CreateStoryEpisodeShootings failed: %v", err)
	}
	if response.GetIsGift() {
		t.Fatal("Photo.CreateStoryEpisodeShootings unexpectedly delivered the created photo as a gift")
	}
	wantPlace := episode.part.GetName() + " " + photoStoryChapterEpisodeLabel(episode.chapter, episode.episode)
	wantEvent := episode.part.GetName()
	if !strings.HasSuffix(wantEvent, "編") {
		wantEvent += "編"
	}
	assertPhotoVariantCreation(
		t, dbCtx, fixture, response.GetPhotos(), response.GetCommonResponse(), wantPlace, wantEvent, episode.storyID,
	)
}

func TestPhotoCardStoryShootingScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	fixture := preparePhotoVariantShooting(t, dbCtx)
	card, storyID, number := selectPhotoCardStoryFixture(t, dbCtx)
	if err := opuser.UpsertUserCard(dbCtx, testConfig.TestUserSuid, &ptransaction.UserCard{
		CardId: card.GetId(),
		Level:  1,
		Rarity: card.GetInitialRarity(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserStory(dbCtx, testConfig.TestUserSuid, &ptransaction.UserStory{
		StoryId:    storyID,
		StatusType: penum.StoryStatusType_StoryStatusType_Read,
	}); err != nil {
		t.Fatal(err)
	}

	client := papi.NewPhotoClient(testConfig.GrpcConn)
	checkCtx, checkCancel := photoGRPCCallContext(t)
	checkResponse, err := client.CheckCardStoryShooting(checkCtx, &papi.PhotoCheckCardStoryShootingRequest{
		CardId: card.GetId(),
		Number: number,
	})
	checkCancel()
	if err != nil {
		t.Fatalf("Photo.CheckCardStoryShooting failed: %v", err)
	}
	assertPhotoCheckResponseIsReadOnly(t, checkResponse.GetCommonResponse())

	createCtx, createCancel := photoGRPCCallContext(t)
	response, err := client.CreateCardStoryShootings(createCtx, &papi.PhotoCreateCardStoryShootingsRequest{
		CardId: card.GetId(),
		Number: number,
		CreateShootingParams: []*papi.PhotoCreateShootingParam{{
			PhotoImageId:    fixture.imageID,
			ItemId:          fixture.filmID,
			MainCharacterId: fixture.characterID,
		}},
		ManualCount: 2,
	})
	createCancel()
	if err != nil {
		t.Fatalf("Photo.CreateCardStoryShootings failed: %v", err)
	}
	if response.GetIsGift() {
		t.Fatal("Photo.CreateCardStoryShootings unexpectedly delivered the created photo as a gift")
	}
	assertPhotoVariantCreation(
		t,
		dbCtx,
		fixture,
		response.GetPhotos(),
		response.GetCommonResponse(),
		card.GetName()+" "+photoEpisodeLabel(number),
		"アイドル編",
		storyID,
	)
}

func TestPhotoExpressionShootingScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	fixture := preparePhotoVariantShooting(t, dbCtx)
	expression := selectPhotoExpressionFixture(t, dbCtx)

	client := papi.NewPhotoClient(testConfig.GrpcConn)
	checkRequest := &papi.PhotoCheckExpressionShootingRequest{
		PhotoExpressionId: expression.expression.GetId(),
		CharacterId:       fixture.characterID,
		CostumeId:         fixture.costumeID,
		HairId:            fixture.hairID,
	}
	checkCtx, checkCancel := photoGRPCCallContext(t)
	checkResponse, err := client.CheckExpressionShooting(checkCtx, checkRequest)
	checkCancel()
	if err != nil {
		t.Fatalf("Photo.CheckExpressionShooting failed: %v", err)
	}
	assertPhotoCheckResponseIsReadOnly(t, checkResponse.GetCommonResponse())

	createCtx, createCancel := photoGRPCCallContext(t)
	response, err := client.CreateExpressionShootings(createCtx, &papi.PhotoCreateExpressionShootingsRequest{
		PhotoExpressionId: checkRequest.GetPhotoExpressionId(),
		CharacterId:       checkRequest.GetCharacterId(),
		CostumeId:         checkRequest.GetCostumeId(),
		HairId:            checkRequest.GetHairId(),
		CreateExpressionShootingParams: []*papi.PhotoCreateExpressionShootingParam{{
			PhotoImageId:        fixture.imageID,
			ItemId:              fixture.filmID,
			StagePositionNumber: expression.position.GetNumber(),
			PhotoPoseId:         expression.pose.GetId(),
			PhotoLookingType:    expression.pose.GetLookingType(),
			MainCharacterId:     fixture.characterID,
			CharacterIds:        []string{fixture.characterID},
			PhotoPoseIds:        []string{expression.pose.GetId()},
			PhotoFacialIds:      []string{""},
			PhotoLookingTypes:   []penum.PhotoLookingType{expression.pose.GetLookingType()},
		}},
		ManualCount: 1,
	})
	createCancel()
	if err != nil {
		t.Fatalf("Photo.CreateExpressionShootings failed: %v", err)
	}
	if response.GetIsGift() {
		t.Fatal("Photo.CreateExpressionShootings unexpectedly delivered the created photo as a gift")
	}
	assertPhotoVariantCreation(
		t,
		dbCtx,
		fixture,
		response.GetPhotos(),
		response.GetCommonResponse(),
		expression.stage.GetName(),
		expression.expression.GetName(),
		"",
	)
}

func TestPhotoExcursionShootingScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	fixture := preparePhotoVariantShooting(t, dbCtx)
	place := selectPhotoExcursionPlaceFixture(t, dbCtx, fixture.characterID)

	client := papi.NewPhotoClient(testConfig.GrpcConn)
	checkRequest := &papi.PhotoCheckExcursionShootingRequest{
		ExcursionPlaceId: place.GetId(),
		CharacterId:      fixture.characterID,
		CostumeId:        fixture.costumeID,
		HairId:           fixture.hairID,
	}
	checkCtx, checkCancel := photoGRPCCallContext(t)
	checkResponse, err := client.CheckExcursionShooting(checkCtx, checkRequest)
	checkCancel()
	if err != nil {
		t.Fatalf("Photo.CheckExcursionShooting failed: %v", err)
	}
	assertPhotoCheckResponseIsReadOnly(t, checkResponse.GetCommonResponse())

	createCtx, createCancel := photoGRPCCallContext(t)
	response, err := client.CreateExcursionShootings(createCtx, &papi.PhotoCreateExcursionShootingsRequest{
		ExcursionPlaceId: checkRequest.GetExcursionPlaceId(),
		CharacterId:      checkRequest.GetCharacterId(),
		CostumeId:        checkRequest.GetCostumeId(),
		HairId:           checkRequest.GetHairId(),
		CreateExcursionShootingParams: []*papi.PhotoCreateExcursionShootingParam{{
			PhotoImageId: fixture.imageID,
			ItemId:       fixture.filmID,
		}},
		ManualCount: 1,
	})
	createCancel()
	if err != nil {
		t.Fatalf("Photo.CreateExcursionShootings failed: %v", err)
	}
	if response.GetIsGift() {
		t.Fatal("Photo.CreateExcursionShootings unexpectedly delivered the created photo as a gift")
	}
	assertPhotoVariantCreation(
		t, dbCtx, fixture, response.GetPhotos(), response.GetCommonResponse(), place.GetName(), place.GetName(), "",
	)
}

const photoVariantFilmSeedAmount int64 = 3

type photoVariantShootingFixture struct {
	characterID           string
	costumeID             string
	hairID                string
	imageID               string
	filmID                string
	reliabilityBefore     int32
	photoShootCountBefore int64
	reservedImageIDs      []string
}

type photoStoryEpisodeFixture struct {
	part    *pmaster.StoryPart
	storyID string
	chapter int32
	route   int32
	episode int32
}

type photoExpressionShootingFixture struct {
	expression *pmaster.PhotoExpression
	position   *pmaster.PhotoExpressionStagePosition
	pose       *pmaster.PhotoPose
	stage      *pmaster.Stage
}

func preparePhotoVariantShooting(t *testing.T, ctx context.Context) photoVariantShootingFixture {
	t.Helper()

	characterIDs, costumeIDs, hairIDs := photoQuestCast(t, ctx)
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(user.GetNextPhotoImageIds()) == 0 {
		t.Fatal("configured test user has no reserved photo image IDs")
	}
	maxManagerLevel, err := opmaster.FindMaxManagerLevel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	user.ManagerLevel = maxManagerLevel.GetLevel()
	user.ManagerExp = maxManagerLevel.GetRequiredExp()
	if err := opuser.UpsertUser(ctx, testConfig.TestUserSuid, user); err != nil {
		t.Fatal(err)
	}

	items, err := opmaster.FindAllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var film *pmaster.Item
	for _, item := range items {
		if item.GetId() == "" || item.GetType() != penum.ItemType_ItemType_PhotoShooting ||
			item.GetIsPhotoShootingCharacterOnly() {
			continue
		}
		if film == nil || item.GetId() < film.GetId() {
			film = item
		}
	}
	if film == nil {
		t.Fatal("master data has no ordinary photo shooting film")
	}
	if err := opuser.UpsertUserItem(ctx, testConfig.TestUserSuid, &ptransaction.UserItem{
		ItemId: film.GetId(),
		Amount: photoVariantFilmSeedAmount,
	}); err != nil {
		t.Fatal(err)
	}
	character, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, characterIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	totalCount, err := opuser.FindUserTotalCount(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	return photoVariantShootingFixture{
		characterID:           characterIDs[0],
		costumeID:             costumeIDs[0],
		hairID:                hairIDs[0],
		imageID:               user.GetNextPhotoImageIds()[0],
		filmID:                film.GetId(),
		reliabilityBefore:     character.GetReliability(),
		photoShootCountBefore: totalCount.GetPhotoShootCount(),
		reservedImageIDs:      slices.Clone(user.GetNextPhotoImageIds()),
	}
}

func selectPhotoStoryEpisodeFixture(t *testing.T, ctx context.Context) photoStoryEpisodeFixture {
	t.Helper()

	parts, err := opmaster.FindAllStoryParts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range parts {
		if part.GetId() == "" || part.GetName() == "" {
			continue
		}
		for _, chapter := range part.GetChapters() {
			if chapter.GetChapter() <= 0 || chapter.GetRoute() <= 0 {
				continue
			}
			for _, episode := range chapter.GetEpisodes() {
				if episode.GetEpisode() <= 0 || episode.GetStoryId() == "" || !episode.GetIsReleased() {
					continue
				}
				if _, err := opmaster.FindStory(ctx, episode.GetStoryId()); errors.Is(err, mongo.ErrNoDocuments) {
					continue
				} else if err != nil {
					t.Fatal(err)
				}
				return photoStoryEpisodeFixture{
					part: part, storyID: episode.GetStoryId(), chapter: chapter.GetChapter(),
					route: chapter.GetRoute(), episode: episode.GetEpisode(),
				}
			}
		}
	}
	t.Fatal("master data has no released story episode suitable for photo shooting")
	return photoStoryEpisodeFixture{}
}

func selectPhotoCardStoryFixture(t *testing.T, ctx context.Context) (*pmaster.Card, string, int32) {
	t.Helper()

	cards, err := opmaster.FindAllCards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, card := range cards {
		if card.GetId() == "" || card.GetName() == "" {
			continue
		}
		for index, story := range card.GetStories() {
			if story.GetStoryId() == "" {
				continue
			}
			if _, err := opmaster.FindStory(ctx, story.GetStoryId()); errors.Is(err, mongo.ErrNoDocuments) {
				continue
			} else if err != nil {
				t.Fatal(err)
			}
			return card, story.GetStoryId(), int32(index + 1)
		}
	}
	t.Fatal("master data has no card story suitable for photo shooting")
	return nil, "", 0
}

func selectPhotoExpressionFixture(t *testing.T, ctx context.Context) photoExpressionShootingFixture {
	t.Helper()

	userExpressions, err := opuser.FindAllUserPhotoExpressions(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	unlockedExpressions := make(map[string]struct{}, len(userExpressions))
	for _, expression := range userExpressions {
		if expression.GetIsUnlocked() {
			unlockedExpressions[expression.GetPhotoExpressionId()] = struct{}{}
		}
	}
	userPoses, err := opuser.FindAllUserPhotoPoses(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	ownedPoses := make(map[string]struct{}, len(userPoses))
	for _, pose := range userPoses {
		ownedPoses[pose.GetPhotoPoseId()] = struct{}{}
	}
	masterPoses, err := opmaster.FindAllPhotoPoses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	posesByID := make(map[string]*pmaster.PhotoPose, len(masterPoses))
	for _, pose := range masterPoses {
		posesByID[pose.GetId()] = pose
	}
	expressions, err := opmaster.FindAllPhotoExpressions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, expression := range expressions {
		if expression.GetId() == "" || expression.GetName() == "" {
			continue
		}
		if _, unlocked := unlockedExpressions[expression.GetId()]; !unlocked {
			continue
		}
		stage, err := opmaster.FindPhotoStage(ctx, expression.GetStageId())
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		positions, err := opmaster.FindPhotoExpressionStagePositions(ctx, expression.GetStageId())
		if err != nil {
			t.Fatal(err)
		}
		for _, position := range positions {
			if position.GetNumber() <= 0 {
				continue
			}
			pose := posesByID[position.GetInitialPhotoPoseId()]
			if pose != nil && pose.GetPoseType() == position.GetPossiblePhotoPoseType() {
				if _, owned := ownedPoses[pose.GetId()]; owned {
					return photoExpressionShootingFixture{
						expression: expression, position: position, pose: pose, stage: stage,
					}
				}
			}
			for _, candidate := range masterPoses {
				if candidate.GetId() == "" || candidate.GetPoseType() != position.GetPossiblePhotoPoseType() {
					continue
				}
				if _, owned := ownedPoses[candidate.GetId()]; owned {
					return photoExpressionShootingFixture{
						expression: expression, position: position, pose: candidate, stage: stage,
					}
				}
			}
		}
	}
	t.Fatal("master and user data have no unlocked expression with an owned compatible pose")
	return photoExpressionShootingFixture{}
}

func selectPhotoExcursionPlaceFixture(t *testing.T, ctx context.Context, characterID string) *pmaster.ExcursionPlace {
	t.Helper()

	settings, err := opmaster.FindExcursionPlaceSettingsByCharacter(ctx, characterID)
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range settings {
		if setting.GetExcursionPlaceId() == "" {
			continue
		}
		place, err := opmaster.FindExcursionPlace(ctx, setting.GetExcursionPlaceId())
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		return place
	}
	t.Fatalf("master data has no excursion place for photographed character %q", characterID)
	return nil
}

func assertPhotoCheckResponseIsReadOnly(t *testing.T, common *papi.CommonResponse) {
	t.Helper()
	if common == nil || !proto.Equal(common.GetUpdatedData(), &papi.UpdatedData{}) ||
		!proto.Equal(common.GetDeletedData(), &papi.DeletedData{}) {
		t.Fatalf("photo shooting check returned mutations: %v", common)
	}
}

func assertPhotoVariantCreation(
	t *testing.T,
	ctx context.Context,
	fixture photoVariantShootingFixture,
	photos []*ptransaction.UserPhoto,
	common *papi.CommonResponse,
	wantPlace string,
	wantEvent string,
	storyID string,
) {
	t.Helper()
	if len(photos) != 1 {
		t.Fatalf("photo shooting returned %d photos for one create parameter", len(photos))
	}
	created := photos[0]
	if created.GetPhotoId() == "" || created.GetAssetId() != fixture.imageID ||
		created.GetImageType() != penum.PhotoImageType_PhotoImageType_User ||
		created.GetFocusCharacterId() != fixture.characterID ||
		created.GetPlaceName() != wantPlace || created.GetEventName() != wantEvent {
		t.Fatalf("photo shooting returned an invalid scene photo: %v", created)
	}
	assertGeneratedPhotoIsMasterValid(t, ctx, created)
	if common == nil || common.GetUpdatedData() == nil || common.GetDeletedData() == nil {
		t.Fatal("photo shooting returned an incomplete common response")
	}
	updated := common.GetUpdatedData()
	if !proto.Equal(photoWithID(updated.GetPhotos(), created.GetPhotoId()), created) {
		t.Fatal("photo shooting omitted the created photo from updatedData")
	}
	if updated.GetUser() == nil || updated.GetTotalCount() == nil {
		t.Fatal("photo shooting omitted updated user or total count")
	}
	updatedCharacter := photoCharacterWithID(updated.GetCharacters(), fixture.characterID)
	if updatedCharacter == nil || updatedCharacter.GetReliability() != fixture.reliabilityBefore+1 {
		t.Fatalf("photo shooting returned an invalid reliability update: %v", updatedCharacter)
	}
	if updated.GetTotalCount().GetPhotoShootCount() != fixture.photoShootCountBefore+1 {
		t.Fatalf(
			"photo shoot count = %d, want %d",
			updated.GetTotalCount().GetPhotoShootCount(), fixture.photoShootCountBefore+1,
		)
	}
	if len(updated.GetUser().GetNextPhotoImageIds()) != len(fixture.reservedImageIDs) ||
		slices.Contains(updated.GetUser().GetNextPhotoImageIds(), fixture.imageID) ||
		!allPhotoImageIDsUnique(updated.GetUser().GetNextPhotoImageIds()) {
		t.Fatalf("photo shooting did not rotate reserved image IDs: %v", updated.GetUser().GetNextPhotoImageIds())
	}
	updatedFilm := photoItemWithID(updated.GetItems(), fixture.filmID)
	if updatedFilm == nil || updatedFilm.GetExpiredTime() != 0 ||
		updatedFilm.GetAmount() != photoVariantFilmSeedAmount-1 {
		t.Fatalf("photo shooting returned an invalid film post-image: %v", updatedFilm)
	}

	storedPhoto, err := opuser.FindUserPhoto(ctx, testConfig.TestUserSuid, created.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	storedCharacter, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, fixture.characterID)
	if err != nil {
		t.Fatal(err)
	}
	storedTotal, err := opuser.FindUserTotalCount(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	storedUser, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(storedPhoto, created) || !proto.Equal(storedCharacter, updatedCharacter) ||
		!proto.Equal(storedTotal, updated.GetTotalCount()) || !proto.Equal(storedUser, updated.GetUser()) {
		t.Fatal("photo shooting response does not match persisted state")
	}
	if amount := photoPermanentItemAmount(t, ctx, fixture.filmID); amount != photoVariantFilmSeedAmount-1 {
		t.Fatalf("persisted shooting film amount = %d, want %d", amount, photoVariantFilmSeedAmount-1)
	}
	if storyID != "" {
		story, err := opuser.FindUserStory(ctx, testConfig.TestUserSuid, storyID)
		if err != nil {
			t.Fatal(err)
		}
		if !story.GetIsPhotoShot() {
			t.Fatalf("photo shooting did not mark story %q as photo-shot", storyID)
		}
		if updatedStory := photoStoryWithID(updated.GetStories(), storyID); updatedStory == nil || !updatedStory.GetIsPhotoShot() {
			t.Fatalf("photo shooting omitted story %q from updatedData", storyID)
		}
	}
}

func photoCharacterWithID(characters []*ptransaction.UserCharacter, characterID string) *ptransaction.UserCharacter {
	for _, character := range characters {
		if character.GetCharacterId() == characterID {
			return character
		}
	}
	return nil
}

func photoStoryWithID(stories []*ptransaction.UserStory, storyID string) *ptransaction.UserStory {
	for _, story := range stories {
		if story.GetStoryId() == storyID {
			return story
		}
	}
	return nil
}

func photoStoryChapterEpisodeLabel(chapter, episode int32) string {
	return fmt.Sprintf("%d章 %d話", chapter, episode)
}

func photoEpisodeLabel(episode int32) string {
	return fmt.Sprintf("%d話", episode)
}

func assertPhotoShootingCatalogMatchesUserData(t *testing.T, ctx context.Context, response *papi.PhotoListShootingResponse) {
	t.Helper()

	activities, err := opuser.FindAllUserPhotoActivities(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	musics, err := opuser.FindAllUserPhotoMusics(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	stages, err := opuser.FindAllUserPhotoStages(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	expressions, err := opuser.FindAllUserPhotoExpressions(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	masterActivities, err := opmaster.FindAllPhotoActivities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	masterMusics, err := opmaster.FindAllPhotoQuestMusics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	masterStages, err := opmaster.FindAllPhotoQuestStages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	masterExpressions, err := opmaster.FindAllPhotoExpressions(ctx)
	if err != nil {
		t.Fatal(err)
	}

	activityStates := func(values []*papi.PhotoActivity) map[string]bool {
		result := make(map[string]bool, len(values))
		for _, value := range values {
			result[value.GetPhotoActivityId()] = value.GetIsUnlocked()
		}
		return result
	}
	musicStates := func(values []*papi.PhotoMusic) map[string]bool {
		result := make(map[string]bool, len(values))
		for _, value := range values {
			result[value.GetMusicId()] = value.GetIsUnlocked()
		}
		return result
	}
	stageStates := func(values []*papi.PhotoStage) map[string]bool {
		result := make(map[string]bool, len(values))
		for _, value := range values {
			result[value.GetStageId()] = value.GetIsUnlocked()
		}
		return result
	}
	expressionStates := func(values []*papi.PhotoExpression) map[string]bool {
		result := make(map[string]bool, len(values))
		for _, value := range values {
			result[value.GetPhotoExpressionId()] = value.GetIsUnlocked()
		}
		return result
	}
	activityIDs := make(map[string]struct{}, len(masterActivities))
	for _, value := range masterActivities {
		activityIDs[value.GetId()] = struct{}{}
	}
	musicIDs := make(map[string]struct{}, len(masterMusics))
	for _, value := range masterMusics {
		musicIDs[value.GetId()] = struct{}{}
	}
	stageIDs := make(map[string]struct{}, len(masterStages))
	for _, value := range masterStages {
		stageIDs[value.GetId()] = struct{}{}
	}
	expressionIDs := make(map[string]struct{}, len(masterExpressions))
	for _, value := range masterExpressions {
		expressionIDs[value.GetId()] = struct{}{}
	}

	if !maps.Equal(activityStates(response.GetActivities()), filterPhotoStateMap(activityStates(activities), activityIDs)) ||
		!maps.Equal(musicStates(response.GetMusics()), filterPhotoStateMap(musicStates(musics), musicIDs)) ||
		!maps.Equal(stageStates(response.GetStages()), filterPhotoStateMap(stageStates(stages), stageIDs)) ||
		!maps.Equal(expressionStates(response.GetExpressions()), filterPhotoStateMap(expressionStates(expressions), expressionIDs)) {
		t.Fatal("Photo.ListShooting response is inconsistent with current user shooting data")
	}
}

func photoQuestCast(t *testing.T, ctx context.Context) (characterIDs, costumeIDs, hairIDs []string) {
	t.Helper()

	characters, err := opuser.FindAllUserCharacters(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	ownedCostumes, err := opuser.FindAllUserCostumes(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	ownedHairs, err := opuser.FindAllUserHairs(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	costumeSet := make(map[string]struct{}, len(ownedCostumes))
	for _, costume := range ownedCostumes {
		costumeSet[costume.GetCostumeId()] = struct{}{}
	}
	hairSet := make(map[string]struct{}, len(ownedHairs))
	for _, hair := range ownedHairs {
		hairSet[hair.GetHairId()] = struct{}{}
	}

	for _, character := range characters {
		costumeID := character.GetFavoriteCostumeId()
		if _, ok := costumeSet[costumeID]; !ok {
			continue
		}
		masterCostume, err := opmaster.FindCostume(ctx, costumeID)
		if err != nil {
			t.Fatal(err)
		}
		hairID := masterCostume.GetDefaultHairId()
		if masterCostume.GetCharacterId() != character.GetCharacterId() {
			continue
		}
		if _, ok := hairSet[hairID]; !ok {
			continue
		}
		characterIDs = append(characterIDs, character.GetCharacterId())
		costumeIDs = append(costumeIDs, costumeID)
		hairIDs = append(hairIDs, hairID)
		if len(characterIDs) == 5 {
			break
		}
	}
	if len(characterIDs) != 5 {
		t.Fatalf("configured test user has only %d complete quest shooting cast entries", len(characterIDs))
	}
	return characterIDs, costumeIDs, hairIDs
}

func firstUnlockedPhotoMusic(values []*papi.PhotoMusic) *papi.PhotoMusic {
	for _, value := range values {
		if value.GetIsUnlocked() {
			return value
		}
	}
	return nil
}

func firstUnlockedPhotoStage(values []*papi.PhotoStage) *papi.PhotoStage {
	for _, value := range values {
		if value.GetIsUnlocked() {
			return value
		}
	}
	return nil
}

func assertGeneratedPhotoIsMasterValid(t *testing.T, ctx context.Context, photo *ptransaction.UserPhoto) {
	t.Helper()
	if photo.GetRarity() < 1 || photo.GetRarity() > 5 {
		t.Fatalf("generated photo rarity %d is outside the documented 1..5 range", photo.GetRarity())
	}
	if _, err := opmaster.FindPhotoRarityLevel(ctx, photo.GetRarity(), photo.GetLevel()); err != nil {
		t.Fatalf("generated photo has no master rarity/level row: rarity=%d level=%d: %v", photo.GetRarity(), photo.GetLevel(), err)
	}
	if len(photo.GetAbilities()) == 0 {
		t.Fatal("generated photo has no abilities")
	}
	seen := make(map[string]struct{}, len(photo.GetAbilities()))
	for _, ability := range photo.GetAbilities() {
		if ability.GetPhotoAbilityId() == "" || ability.GetEffectValue() <= 0 || ability.GetGrade() < 1 || ability.GetGrade() > 10 {
			t.Fatalf("generated photo contains an invalid ability: %v", ability)
		}
		if _, duplicate := seen[ability.GetPhotoAbilityId()]; duplicate {
			t.Fatalf("generated photo contains duplicate ability %q", ability.GetPhotoAbilityId())
		}
		seen[ability.GetPhotoAbilityId()] = struct{}{}
		if _, err := opmaster.FindPhotoAbility(ctx, ability.GetPhotoAbilityId()); err != nil {
			t.Fatalf("generated photo ability %q is absent from master data: %v", ability.GetPhotoAbilityId(), err)
		}
	}
}

func assertPhotoIdentityExceptRandomOptions(t *testing.T, before, after *ptransaction.UserPhoto) {
	t.Helper()
	want := proto.Clone(before).(*ptransaction.UserPhoto)
	got := proto.Clone(after).(*ptransaction.UserPhoto)
	want.Abilities = nil
	got.Abilities = nil
	want.CharacterIds = nil
	got.CharacterIds = nil
	if !proto.Equal(want, got) {
		t.Fatalf("Photo.Reroll changed fields outside randomized options: before=%v after=%v", before, after)
	}
}

func assertRerolledPhotoOptionsAreMasterValid(t *testing.T, ctx context.Context, photo *ptransaction.UserPhoto) {
	t.Helper()
	assertGeneratedPhotoIsMasterValid(t, ctx, photo)
	if len(photo.GetCharacterIds()) > 1 {
		t.Fatalf("rerolled photo has multiple character restrictions: %v", photo.GetCharacterIds())
	}
	sets, err := opmaster.FindAllPhotoAbilitySets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantAbilityIDs := make([]string, 0, len(photo.GetAbilities()))
	for _, ability := range photo.GetAbilities() {
		wantAbilityIDs = append(wantAbilityIDs, ability.GetPhotoAbilityId())
	}
	for _, set := range sets {
		if set.GetPhotoRarity() != photo.GetRarity() || !slices.Equal(set.GetPhotoAbilityIds(), wantAbilityIDs) {
			continue
		}
		if len(photo.GetCharacterIds()) == 0 {
			if !set.GetLimited() && !strings.Contains(set.GetId(), "recipe") {
				return
			}
			continue
		}
		shortID := strings.TrimPrefix(photo.GetCharacterIds()[0], "char-")
		if set.GetLimited() && strings.Contains(set.GetId(), "-"+shortID+"-idol-") {
			return
		}
	}
	t.Fatalf("rerolled options do not match a compatible master ability set: %v", photo)
}

func photoPermanentItemAmount(t *testing.T, ctx context.Context, itemID string) int64 {
	t.Helper()
	items, err := opuser.FindUserItems(ctx, testConfig.TestUserSuid, itemID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.GetExpiredTime() == 0 {
			return item.GetAmount()
		}
	}
	return 0
}

func photoWithID(photos []*ptransaction.UserPhoto, photoID string) *ptransaction.UserPhoto {
	for _, photo := range photos {
		if photo.GetPhotoId() == photoID {
			return photo
		}
	}
	return nil
}

func photoGiftForResource(gifts []*ptransaction.UserGift, photoID string) *ptransaction.UserGift {
	for _, gift := range gifts {
		if gift.GetResourceType() == penum.ResourceType_ResourceType_PhotoGift && gift.GetResourceId() == photoID {
			return gift
		}
	}
	return nil
}

func photoGiftNotification(notifications []*ptransaction.UserNotification) *ptransaction.UserNotification {
	for _, notification := range notifications {
		if notification.GetNotificationType() == penum.NotificationType_NotificationType_Gift {
			return notification
		}
	}
	return nil
}

func allPhotoImageIDsUnique(ids []string) bool {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return false
		}
		if _, duplicate := seen[id]; duplicate {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func photoGRPCCallContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cancel
}

func filterPhotoStateMap(states map[string]bool, validIDs map[string]struct{}) map[string]bool {
	result := make(map[string]bool, len(states))
	for id, state := range states {
		if _, valid := validIDs[id]; valid {
			result[id] = state
		}
	}
	return result
}

type photoRetouchFixture struct {
	recipe           *pmaster.PhotoRecipe
	abilitySet       *pmaster.PhotoAbilitySet
	abilities        map[string]*pmaster.PhotoAbility
	consumption      *pmaster.Consumption
	resultLevels     []int32
	focusCharacterID string
}

func selectPhotoRetouchFixture(t *testing.T, ctx context.Context) photoRetouchFixture {
	t.Helper()
	characters, err := opuser.FindAllUserCharacters(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(characters) == 0 {
		t.Fatal("configured test user has no characters for a retouch fixture")
	}
	recipes, err := opmaster.FindAllPhotoRecipes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, recipe := range recipes {
		if recipe.GetId() == "" || recipe.GetMinPhotoLevel() <= 0 ||
			recipe.GetMaxPhotoLevel() < recipe.GetMinPhotoLevel() ||
			recipe.GetPhotoAbilitySetId() == "" || recipe.GetRetouchConsumptionId() == "" {
			continue
		}
		focusCharacterID := characters[0].GetCharacterId()
		consumption, err := opmaster.FindConsumption(ctx, recipe.GetRetouchConsumptionId())
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if !isSupportedPhotoItemConsumption(consumption) {
			continue
		}
		abilitySet, err := opmaster.FindPhotoAbilitySet(ctx, recipe.GetPhotoAbilitySetId())
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if abilitySet.GetPhotoRarity() < 1 || abilitySet.GetPhotoRarity() > 5 {
			continue
		}
		abilities, valid := testPhotoAbilityMasters(ctx, t, abilitySet)
		if !valid {
			continue
		}
		levels, err := opmaster.FindPhotoRarityLevels(ctx, abilitySet.GetPhotoRarity())
		if err != nil {
			t.Fatal(err)
		}
		resultLevels := make([]int32, 0, len(levels))
		for _, candidate := range levels {
			if candidate.GetLevel() < recipe.GetMinPhotoLevel() || candidate.GetLevel() > recipe.GetMaxPhotoLevel() {
				continue
			}
			validLevel := true
			for _, abilityID := range abilitySet.GetPhotoAbilityIds() {
				master := abilities[abilityID]
				for _, grade := range master.GetPhotoAbilityGrades() {
					if _, ok := testPhotoAbilityEffectValue(master, candidate.GetLevel(), grade.GetGrade()); !ok {
						validLevel = false
						break
					}
				}
				if !validLevel {
					break
				}
			}
			if validLevel {
				resultLevels = append(resultLevels, candidate.GetLevel())
			}
		}
		if len(resultLevels) == 0 {
			continue
		}
		return photoRetouchFixture{
			recipe:           recipe,
			abilitySet:       abilitySet,
			abilities:        abilities,
			consumption:      consumption,
			resultLevels:     resultLevels,
			focusCharacterID: focusCharacterID,
		}
	}
	t.Fatal("master data has no photo recipe suitable for the retouch scenario")
	return photoRetouchFixture{}
}

func isSupportedPhotoItemConsumption(consumption *pmaster.Consumption) bool {
	if consumption == nil || len(consumption.GetResources()) == 0 {
		return false
	}
	for _, resource := range consumption.GetResources() {
		if resource.GetResourceType() != penum.ResourceType_ResourceType_Item ||
			len(resource.GetResourceIds()) != 1 || resource.GetResourceIds()[0] == "" || resource.GetAmount() <= 0 {
			return false
		}
	}
	return true
}

func photoConsumptionItemCosts(t *testing.T, consumption *pmaster.Consumption) map[string]int64 {
	t.Helper()
	if !isSupportedPhotoItemConsumption(consumption) {
		t.Fatal("photo consumption fixture contains unsupported resources")
	}
	costs := make(map[string]int64, len(consumption.GetResources()))
	for _, resource := range consumption.GetResources() {
		costs[resource.GetResourceIds()[0]] += int64(resource.GetAmount())
	}
	return costs
}

func testPhotoAbilityMasters(
	ctx context.Context,
	t *testing.T,
	set *pmaster.PhotoAbilitySet,
) (map[string]*pmaster.PhotoAbility, bool) {
	t.Helper()
	if set == nil || len(set.GetPhotoAbilityIds()) == 0 {
		return nil, false
	}
	masters, err := opmaster.FindPhotoAbilities(ctx, set.GetPhotoAbilityIds())
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]*pmaster.PhotoAbility, len(masters))
	for _, master := range masters {
		byID[master.GetId()] = master
	}
	seen := make(map[string]struct{}, len(set.GetPhotoAbilityIds()))
	for _, abilityID := range set.GetPhotoAbilityIds() {
		master := byID[abilityID]
		if master == nil || len(master.GetPhotoAbilityLevels()) == 0 || len(master.GetPhotoAbilityGrades()) == 0 {
			return nil, false
		}
		if _, duplicate := seen[abilityID]; duplicate {
			return nil, false
		}
		seen[abilityID] = struct{}{}
	}
	return byID, true
}

func buildTestPhotoAbilities(
	t *testing.T,
	set *pmaster.PhotoAbilitySet,
	masters map[string]*pmaster.PhotoAbility,
	level int32,
) []*ptransaction.UserPhotoAbility {
	t.Helper()
	result := make([]*ptransaction.UserPhotoAbility, 0, len(set.GetPhotoAbilityIds()))
	for _, abilityID := range set.GetPhotoAbilityIds() {
		master := masters[abilityID]
		if master == nil || len(master.GetPhotoAbilityGrades()) == 0 {
			t.Fatalf("photo ability fixture is missing master data for %q", abilityID)
		}
		grade := master.GetPhotoAbilityGrades()[0].GetGrade()
		effect, ok := testPhotoAbilityEffectValue(master, level, grade)
		if !ok {
			t.Fatalf("photo ability fixture %q lacks level %d or grade %d", abilityID, level, grade)
		}
		result = append(result, &ptransaction.UserPhotoAbility{
			PhotoAbilityId: abilityID,
			EffectValue:    effect,
			Grade:          grade,
			IsAvailable:    true,
		})
	}
	return result
}

func assertTestPhotoAbilitiesMatchMaster(
	t *testing.T,
	abilities []*ptransaction.UserPhotoAbility,
	set *pmaster.PhotoAbilitySet,
	masters map[string]*pmaster.PhotoAbility,
	level int32,
) {
	t.Helper()
	if len(abilities) != len(set.GetPhotoAbilityIds()) {
		t.Fatalf("photo has %d retouched abilities, want %d", len(abilities), len(set.GetPhotoAbilityIds()))
	}
	for index, ability := range abilities {
		wantID := set.GetPhotoAbilityIds()[index]
		wantEffect, ok := testPhotoAbilityEffectValue(masters[wantID], level, ability.GetGrade())
		if !ok || ability.GetPhotoAbilityId() != wantID || ability.GetEffectValue() != wantEffect || !ability.GetIsAvailable() {
			t.Fatalf("retouched ability at index %d is inconsistent with master data: %v", index, ability)
		}
	}
}

func testPhotoAbilityEffectValue(master *pmaster.PhotoAbility, level, grade int32) (int64, bool) {
	if master == nil {
		return 0, false
	}
	var selectedLevel *pmaster.PhotoAbilityLevel
	for _, candidate := range master.GetPhotoAbilityLevels() {
		if candidate.GetLevel() <= level && (selectedLevel == nil || candidate.GetLevel() > selectedLevel.GetLevel()) {
			selectedLevel = candidate
		}
	}
	if selectedLevel == nil {
		for _, candidate := range master.GetPhotoAbilityLevels() {
			if selectedLevel == nil || candidate.GetLevel() < selectedLevel.GetLevel() {
				selectedLevel = candidate
			}
		}
	}
	var selectedGrade *pmaster.PhotoAbilityGrade
	for _, candidate := range master.GetPhotoAbilityGrades() {
		if candidate.GetGrade() == grade {
			selectedGrade = candidate
			break
		}
	}
	if selectedLevel == nil || selectedGrade == nil {
		return 0, false
	}
	return int64(selectedLevel.GetValue())*int64(selectedGrade.GetBonusPermil())/1000 +
		int64(selectedGrade.GetBonusValue()), true
}

func selectPhotoRecipeSaleFixture(t *testing.T, ctx context.Context) (*pmaster.PhotoRecipe, *pmaster.RewardResource) {
	t.Helper()
	recipes, err := opmaster.FindAllPhotoRecipes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, recipe := range recipes {
		if recipe.GetId() == "" || recipe.GetSaleRewardId() == "" {
			continue
		}
		reward, err := opmaster.FindReward(ctx, recipe.GetSaleRewardId())
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(reward.GetResources()) != 1 {
			continue
		}
		resource := reward.GetResources()[0]
		if resource.GetResourceType() != penum.ResourceType_ResourceType_Item ||
			resource.GetResourceId() == "" || resource.GetAmount() <= 0 {
			continue
		}
		if _, err := opmaster.FindItem(ctx, resource.GetResourceId()); errors.Is(err, mongo.ErrNoDocuments) {
			continue
		} else if err != nil {
			t.Fatal(err)
		}
		return recipe, resource
	}
	t.Fatal("master data has no photo recipe with a single item sale reward")
	return nil, nil
}

type dreamPhotoFixture struct {
	abilitySet *pmaster.PhotoAbilitySet
	abilities  map[string]*pmaster.PhotoAbility
	level      int32
}

type retouchedDreamBaseEnhanceFixture struct {
	recipe             *pmaster.PhotoRecipe
	abilitySet         *pmaster.PhotoAbilitySet
	abilities          map[string]*pmaster.PhotoAbility
	retouchConsumption *pmaster.Consumption
	materialTarget     *pmaster.PhotoRarityLevel
	currentLevel       int32
	targetLevel        int32
}

func selectRetouchedDreamBaseEnhanceFixture(t *testing.T, ctx context.Context) retouchedDreamBaseEnhanceFixture {
	t.Helper()
	managerLevel, err := opmaster.FindMaxManagerLevel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recipes, err := opmaster.FindAllPhotoRecipes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, recipe := range recipes {
		if recipe.GetId() == "" || recipe.GetPhotoAbilitySetId() == "" ||
			recipe.GetPhotoRetouchEnhanceGroupId() == "" {
			continue
		}
		set, err := opmaster.FindPhotoAbilitySet(ctx, recipe.GetPhotoAbilitySetId())
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		abilities, valid := testPhotoAbilityMasters(ctx, t, set)
		if !valid {
			continue
		}
		levels, err := opmaster.FindPhotoRarityLevels(ctx, set.GetPhotoRarity())
		if err != nil {
			t.Fatal(err)
		}
		byLevel := make(map[int32]*pmaster.PhotoRarityLevel, len(levels))
		for _, level := range levels {
			byLevel[level.GetLevel()] = level
		}
		for _, current := range levels {
			currentLevel := current.GetLevel()
			targetLevel := currentLevel + 5
			materialTarget := byLevel[targetLevel]
			if currentLevel < recipe.GetMinPhotoLevel() || currentLevel > recipe.GetMaxPhotoLevel() ||
				targetLevel > managerLevel.GetMaxPhotoEnhanceLevel() || materialTarget == nil ||
				materialTarget.GetEnhanceRequiredItem() <= 0 {
				continue
			}
			enhance, err := opmaster.FindPhotoRetouchEnhance(
				ctx, recipe.GetPhotoRetouchEnhanceGroupId(), targetLevel,
			)
			if errors.Is(err, mongo.ErrNoDocuments) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			consumption, err := opmaster.FindConsumption(ctx, enhance.GetConsumptionId())
			if errors.Is(err, mongo.ErrNoDocuments) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if !isSupportedPhotoItemConsumption(consumption) {
				continue
			}
			hasInk := false
			for _, resource := range consumption.GetResources() {
				if strings.HasPrefix(resource.GetResourceIds()[0], "item-photo_ink") {
					hasInk = true
					break
				}
			}
			if !hasInk {
				continue
			}
			validEffects := true
			for _, abilityID := range set.GetPhotoAbilityIds() {
				master := abilities[abilityID]
				grade := master.GetPhotoAbilityGrades()[0].GetGrade()
				if _, ok := testPhotoAbilityEffectValue(master, currentLevel, grade); !ok {
					validEffects = false
					break
				}
				if _, ok := testPhotoAbilityEffectValue(master, targetLevel, grade); !ok {
					validEffects = false
					break
				}
			}
			if !validEffects {
				continue
			}
			return retouchedDreamBaseEnhanceFixture{
				recipe:             recipe,
				abilitySet:         set,
				abilities:          abilities,
				retouchConsumption: consumption,
				materialTarget:     materialTarget,
				currentLevel:       currentLevel,
				targetLevel:        targetLevel,
			}
		}
	}
	t.Fatal("master data has no retouched DreamBase enhancement fixture")
	return retouchedDreamBaseEnhanceFixture{}
}

func selectDreamPhotoFixture(t *testing.T, ctx context.Context) dreamPhotoFixture {
	t.Helper()
	sets, err := opmaster.FindAllPhotoAbilitySets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, set := range sets {
		if set.GetPhotoRarity() < 1 || set.GetPhotoRarity() > 5 {
			continue
		}
		abilities, valid := testPhotoAbilityMasters(ctx, t, set)
		if !valid {
			continue
		}
		rarityLevels, err := opmaster.FindPhotoRarityLevels(ctx, set.GetPhotoRarity())
		if err != nil {
			t.Fatal(err)
		}
		validRarityLevels := make(map[int32]struct{}, len(rarityLevels))
		for _, rarityLevel := range rarityLevels {
			validRarityLevels[rarityLevel.GetLevel()] = struct{}{}
		}
		for _, candidate := range abilities[set.GetPhotoAbilityIds()[0]].GetPhotoAbilityLevels() {
			level := candidate.GetLevel()
			if level <= 0 {
				continue
			}
			if _, exists := validRarityLevels[level]; !exists {
				continue
			}
			if _, exists := validRarityLevels[level+5]; !exists {
				continue
			}
			for _, abilityID := range set.GetPhotoAbilityIds() {
				master := abilities[abilityID]
				grade := master.GetPhotoAbilityGrades()[0].GetGrade()
				if !photoAbilityHasExactLevel(master, level) || !photoAbilityHasExactLevel(master, level+5) {
					valid = false
					break
				}
				if _, ok := testPhotoAbilityEffectValue(master, level, grade); !ok {
					valid = false
					break
				}
			}
			if valid {
				return dreamPhotoFixture{abilitySet: set, abilities: abilities, level: level}
			}
			valid = true
		}
	}
	t.Fatal("master data has no ability set suitable for a dream photo level-info scenario")
	return dreamPhotoFixture{}
}

func photoAbilityHasExactLevel(master *pmaster.PhotoAbility, level int32) bool {
	for _, candidate := range master.GetPhotoAbilityLevels() {
		if candidate.GetLevel() == level {
			return true
		}
	}
	return false
}

func photoRecipeWithID(recipes []*ptransaction.UserPhotoRecipe, recipeID string) *ptransaction.UserPhotoRecipe {
	for _, recipe := range recipes {
		if recipe.GetPhotoRecipeId() == recipeID {
			return recipe
		}
	}
	return nil
}

func photoItemWithID(items []*ptransaction.UserItem, itemID string) *ptransaction.UserItem {
	for _, item := range items {
		if item.GetItemId() == itemID && item.GetExpiredTime() == 0 {
			return item
		}
	}
	return nil
}
