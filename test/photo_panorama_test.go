package test

import (
	"context"
	"slices"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"google.golang.org/protobuf/proto"
)

func TestPhotoPanoramaScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	client := papi.NewPhotoPanoramaClient(testConfig.GrpcConn)

	listCtx, listCancel := photoGRPCCallContext(t)
	listResponse, err := client.List(listCtx, &papi.Empty{})
	listCancel()
	if err != nil {
		t.Fatalf("PhotoPanorama.List failed: %v", err)
	}
	if listResponse.GetCommonResponse().GetUpdatedData() == nil ||
		listResponse.GetCommonResponse().GetDeletedData() == nil {
		t.Fatal("PhotoPanorama.List returned an incomplete common response")
	}
	if len(listResponse.GetMusics()) == 0 {
		t.Fatal("PhotoPanorama.List returned no music")
	}

	masterMusics, err := opmaster.FindAllMusics(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	masterMusicByID := make(map[string]*pmaster.Music, len(masterMusics))
	for _, music := range masterMusics {
		masterMusicByID[music.GetId()] = music
	}

	var selected *papi.PhotoPanoramaMusic
	var selectedMaster *pmaster.Music
	var selectedStage *pmaster.Stage
	seenMusicIDs := make(map[string]struct{}, len(listResponse.GetMusics()))
	seenOrders := make(map[int32]struct{}, len(listResponse.GetMusics()))
	for _, music := range listResponse.GetMusics() {
		if music == nil || music.GetMusicId() == "" || music.GetStageId() == "" ||
			music.GetAssetId() == "" || music.GetOrder() <= 0 ||
			music.GetUnlockConditionId() == "" || music.GetViewConditionId() == "" {
			t.Fatalf("PhotoPanorama.List returned incomplete music data: %v", music)
		}
		if _, duplicate := seenMusicIDs[music.GetMusicId()]; duplicate {
			t.Fatalf("PhotoPanorama.List returned duplicate music %q", music.GetMusicId())
		}
		seenMusicIDs[music.GetMusicId()] = struct{}{}
		if _, duplicate := seenOrders[music.GetOrder()]; duplicate {
			t.Fatalf("PhotoPanorama.List returned duplicate order %d", music.GetOrder())
		}
		seenOrders[music.GetOrder()] = struct{}{}

		master := masterMusicByID[music.GetMusicId()]
		if master == nil || !master.GetIs360LiveExist() || music.GetAssetId() != master.GetAssetId() {
			t.Fatalf("PhotoPanorama.List music is inconsistent with master data: response=%v master=%v", music, master)
		}
		stage, err := opmaster.FindPhotoStage(dbCtx, music.GetStageId())
		if err != nil {
			t.Fatalf("PhotoPanorama.List stage %q is absent from master data: %v", music.GetStageId(), err)
		}
		if len(stage.GetCameraPositions()) == 0 {
			t.Fatalf("PhotoPanorama.List stage %q has no panorama camera positions", music.GetStageId())
		}
		if selected == nil && music.GetIsUnlocked() && master.GetDisplayCharacterAmount() > 0 &&
			master.GetDisplayCharacterAmount() <= 5 {
			selected = music
			selectedMaster = master
			selectedStage = stage
		}
	}
	if selected == nil {
		t.Fatal("configured test user has no unlocked panorama music usable by a five-character cast")
	}

	characterIDs, costumeIDs, hairIDs := photoQuestCast(t, dbCtx)
	castSize := int(selectedMaster.GetDisplayCharacterAmount())
	characterIDs = slices.Clone(characterIDs[:castSize])
	costumeIDs = slices.Clone(costumeIDs[:castSize])
	hairIDs = slices.Clone(hairIDs[:castSize])
	checkRequest := &papi.PhotoPanoramaCheckShootingRequest{
		PhotoMusicId: selected.GetMusicId(),
		CharacterIds: characterIDs,
		CostumeIds:   costumeIDs,
		HairIds:      hairIDs,
	}
	checkCtx, checkCancel := photoGRPCCallContext(t)
	checkResponse, err := client.CheckShooting(checkCtx, checkRequest)
	checkCancel()
	if err != nil {
		t.Fatalf("PhotoPanorama.CheckShooting failed: %v", err)
	}
	if checkResponse.GetCommonResponse().GetUpdatedData() == nil ||
		checkResponse.GetCommonResponse().GetDeletedData() == nil {
		t.Fatal("PhotoPanorama.CheckShooting returned an incomplete common response")
	}

	userBefore, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
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
	focusCharacterBefore, err := opuser.FindUserCharacter(dbCtx, testConfig.TestUserSuid, characterIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	totalBefore, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	panoramaMissionIDs := activatePhotoPanoramaMissions(t, dbCtx)
	photosBefore, err := opuser.FindAllUserPhotos(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}

	createRequest := &papi.PhotoPanoramaCreateShootingsRequest{
		PhotoMusicId: selected.GetMusicId(),
		CharacterIds: slices.Clone(characterIDs),
		CostumeIds:   slices.Clone(costumeIDs),
		HairIds:      slices.Clone(hairIDs),
		CreateShootingParams: []*papi.PhotoCreateShootingParam{{
			PhotoImageId:    usedImageID,
			ItemId:          filmItemID,
			MainCharacterId: characterIDs[0],
		}},
		ManualCount:              1,
		CameraTypes:              []penum.PanoramaCameraType{penum.PanoramaCameraType_PanoramaCameraType_StageCamera},
		StartCharacterId:         "",
		StartCameraPositionIndex: selectedStage.GetCameraPositions()[0].GetIndex(),
	}

	invalidRequest := proto.Clone(createRequest).(*papi.PhotoPanoramaCreateShootingsRequest)
	invalidRequest.CameraTypes = nil
	invalidCtx, invalidCancel := photoGRPCCallContext(t)
	_, err = client.CreateShootings(invalidCtx, invalidRequest)
	invalidCancel()
	if err == nil {
		t.Fatal("PhotoPanorama.CreateShootings accepted a cameraTypes length mismatch")
	}
	assertPhotoPanoramaStateUnchanged(
		t, dbCtx, filmItemID, filmSeedAmount, characterIDs[0], focusCharacterBefore.GetReliability(),
		totalBefore.GetPhotoShootCount(), len(photosBefore), reservedImageIDs, panoramaMissionIDs,
	)

	createCtx, createCancel := photoGRPCCallContext(t)
	createResponse, err := client.CreateShootings(createCtx, createRequest)
	createCancel()
	if err != nil {
		t.Fatalf("PhotoPanorama.CreateShootings failed: %v", err)
	}
	if createResponse.GetIsGift() {
		t.Fatal("the first panorama photo of a fresh user was unexpectedly delivered as a gift")
	}
	if len(createResponse.GetPhotos()) != 1 {
		t.Fatalf("PhotoPanorama.CreateShootings returned %d photos for one create parameter", len(createResponse.GetPhotos()))
	}
	created := createResponse.GetPhotos()[0]
	if created.GetPhotoId() == "" || created.GetAssetId() != usedImageID ||
		created.GetImageType() != penum.PhotoImageType_PhotoImageType_User ||
		created.GetFocusCharacterId() != characterIDs[0] {
		t.Fatalf("PhotoPanorama.CreateShootings returned an invalid photo identity: %v", created)
	}
	if created.GetPlaceName() != selectedStage.GetName() || created.GetEventName() != "360°LIVE " {
		t.Fatalf("PhotoPanorama.CreateShootings returned the wrong scene: place=%q event=%q", created.GetPlaceName(), created.GetEventName())
	}
	assertGeneratedPhotoIsMasterValid(t, dbCtx, created)

	updated := createResponse.GetCommonResponse().GetUpdatedData()
	if updated == nil || createResponse.GetCommonResponse().GetDeletedData() == nil ||
		updated.GetUser() == nil || updated.GetTotalCount() == nil {
		t.Fatal("PhotoPanorama.CreateShootings returned an incomplete common response")
	}
	updatedPhoto := photoWithID(updated.GetPhotos(), created.GetPhotoId())
	if updatedPhoto == nil || !proto.Equal(updatedPhoto, created) {
		t.Fatal("PhotoPanorama.CreateShootings omitted the created photo from updatedData")
	}

	stored, err := opuser.FindUserPhoto(dbCtx, testConfig.TestUserSuid, created.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(stored, created) {
		t.Fatalf("stored panorama photo differs from response: stored=%v response=%v", stored, created)
	}
	if amount := photoPermanentItemAmount(t, dbCtx, filmItemID); amount != filmSeedAmount-1 {
		t.Fatalf("panorama shooting film amount = %d, want %d", amount, filmSeedAmount-1)
	}
	focusCharacterAfter, err := opuser.FindUserCharacter(dbCtx, testConfig.TestUserSuid, characterIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if focusCharacterAfter.GetReliability() != focusCharacterBefore.GetReliability()+1 {
		t.Fatalf("panorama focus character reliability = %d, want %d", focusCharacterAfter.GetReliability(), focusCharacterBefore.GetReliability()+1)
	}
	totalAfter, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if totalAfter.GetPhotoShootCount() != totalBefore.GetPhotoShootCount()+1 {
		t.Fatalf("photo shoot count = %d, want %d", totalAfter.GetPhotoShootCount(), totalBefore.GetPhotoShootCount()+1)
	}
	for _, missionID := range panoramaMissionIDs {
		mission, err := findUserMission(dbCtx, missionID)
		if err != nil || mission.GetProgress() != 1 {
			t.Fatalf("panorama mission %q progress = %v, want 1: %v", missionID, mission, err)
		}
		if updatedMission := findUserMissionByID(updated.GetMissions(), missionID); updatedMission == nil ||
			updatedMission.GetProgress() != 1 {
			t.Fatalf("PhotoPanorama.CreateShootings omitted panorama mission %q: %v", missionID, updatedMission)
		}
	}
	userAfter, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(userAfter.GetNextPhotoImageIds()) != len(reservedImageIDs) ||
		slices.Contains(userAfter.GetNextPhotoImageIds(), usedImageID) ||
		!allPhotoImageIDsUnique(userAfter.GetNextPhotoImageIds()) {
		t.Fatalf("PhotoPanorama.CreateShootings did not rotate reserved image IDs correctly: before=%v after=%v", reservedImageIDs, userAfter.GetNextPhotoImageIds())
	}

	getCtx, getCancel := photoGRPCCallContext(t)
	getResponse, err := papi.NewUserClient(testConfig.GrpcConn).Get(getCtx, &papi.Empty{})
	getCancel()
	if err != nil {
		t.Fatalf("User.Get after panorama shooting failed: %v", err)
	}
	if !proto.Equal(photoWithID(getResponse.GetPhotos(), created.GetPhotoId()), stored) ||
		getResponse.GetTotalCount().GetPhotoShootCount() != totalAfter.GetPhotoShootCount() ||
		!slices.Equal(getResponse.GetUser().GetNextPhotoImageIds(), userAfter.GetNextPhotoImageIds()) {
		t.Fatal("User.Get did not expose the persisted panorama shooting state")
	}
}

func assertPhotoPanoramaStateUnchanged(
	t *testing.T,
	ctx context.Context,
	filmItemID string,
	filmAmount int64,
	characterID string,
	reliability int32,
	photoShootCount int64,
	photoCount int,
	reservedImageIDs []string,
	panoramaMissionIDs []string,
) {
	t.Helper()

	if amount := photoPermanentItemAmount(t, ctx, filmItemID); amount != filmAmount {
		t.Fatalf("rejected panorama shooting changed film amount to %d, want %d", amount, filmAmount)
	}
	character, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, characterID)
	if err != nil {
		t.Fatal(err)
	}
	if character.GetReliability() != reliability {
		t.Fatalf("rejected panorama shooting changed reliability to %d, want %d", character.GetReliability(), reliability)
	}
	total, err := opuser.FindUserTotalCount(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if total.GetPhotoShootCount() != photoShootCount {
		t.Fatalf("rejected panorama shooting changed photo shoot count to %d, want %d", total.GetPhotoShootCount(), photoShootCount)
	}
	photos, err := opuser.FindAllUserPhotos(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != photoCount {
		t.Fatalf("rejected panorama shooting changed owned photo count to %d, want %d", len(photos), photoCount)
	}
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(user.GetNextPhotoImageIds(), reservedImageIDs) {
		t.Fatalf("rejected panorama shooting changed reserved image IDs: before=%v after=%v", reservedImageIDs, user.GetNextPhotoImageIds())
	}
	for _, missionID := range panoramaMissionIDs {
		mission, err := findUserMission(ctx, missionID)
		if err != nil || mission.GetProgress() != 0 {
			t.Fatalf("rejected panorama shooting changed mission %q: mission=%v err=%v", missionID, mission, err)
		}
	}
}

func activatePhotoPanoramaMissions(t *testing.T, ctx context.Context) []string {
	t.Helper()

	masters, err := opmaster.FindAllMissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantedTypes := map[penum.MissionType]struct{}{
		penum.MissionType_MissionType_PhotoPanoramaShootingCount:      {},
		penum.MissionType_MissionType_PhotoPanoramaShootingTotalCount: {},
	}
	activatedTypes := make(map[penum.MissionType]struct{}, len(wantedTypes))
	missionIDs := make([]string, 0, len(wantedTypes))
	for _, master := range masters {
		if _, wanted := wantedTypes[master.GetMissionType()]; !wanted {
			continue
		}
		if _, activated := activatedTypes[master.GetMissionType()]; activated {
			continue
		}
		var threshold int64
		for _, progress := range master.GetProgresses() {
			if progress.GetThreshold() > 0 && (threshold == 0 || progress.GetThreshold() < threshold) {
				threshold = progress.GetThreshold()
			}
		}
		if threshold == 0 {
			continue
		}
		if err := opuser.UpsertUserMission(ctx, testConfig.TestUserSuid, &ptransaction.UserMission{
			MissionId:        master.GetId(),
			CurrentThreshold: threshold,
			StatusType:       penum.MissionStatusType_MissionStatusType_Progress,
		}); err != nil {
			t.Fatal(err)
		}
		activatedTypes[master.GetMissionType()] = struct{}{}
		missionIDs = append(missionIDs, master.GetId())
	}
	return missionIDs
}
