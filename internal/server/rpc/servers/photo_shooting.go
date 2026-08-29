package servers

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/mcontext"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	reservedPhotoImageIDCount = 20
	photoGiftRetentionMillis  = int64((14 * 24 * time.Hour) / time.Millisecond)
)

type photoCreateParam struct {
	imageID          string
	itemID           string
	focusCharacterID string
}

type photoShootingScene struct {
	placeName                string
	eventName                string
	candidateCharacterIDs    []string
	storyID                  string
	storyShooting            bool
	expressionShooting       bool
	musicID                  string
	specifiedPoseCounts      map[string]int64
	specifiedCharacterCounts map[string]int64
}

type photoCreationResult struct {
	photos  []*ptransaction.UserPhoto
	isGift  bool
	updated *papi.UpdatedData
}

func (s *PhotoServer) CreateShootings(
	ctx context.Context,
	req *papi.PhotoCreateShootingsRequest,
) (*papi.PhotoCreateShootingsResponse, error) {
	if len(req.GetCreateShootingParams()) == 0 {
		return nil, viber.InvalidArgumentf("at least one createShootingParam is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoCreateShootingsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := s.CheckShooting(ctx, &papi.PhotoCheckShootingRequest{
			ActionType:      req.GetActionType(),
			PhotoActivityId: req.GetPhotoActivityId(),
			PhotoMusicId:    req.GetPhotoMusicId(),
			PhotoStageId:    req.GetPhotoStageId(),
			CharacterIds:    req.GetCharacterIds(),
			CostumeIds:      req.GetCostumeIds(),
			HairIds:         req.GetHairIds(),
		}); err != nil {
			return nil, err
		}
		scene, err := normalPhotoScene(ctx, req)
		if err != nil {
			return nil, err
		}
		params := make([]photoCreateParam, 0, len(req.GetCreateShootingParams()))
		for _, param := range req.GetCreateShootingParams() {
			if param == nil {
				return nil, viber.InvalidArgumentf("createShootingParam is required")
			}
			params = append(params, photoCreateParam{
				imageID:          param.GetPhotoImageId(),
				itemID:           param.GetItemId(),
				focusCharacterID: param.GetMainCharacterId(),
			})
		}
		result, err := createUserPhotoShootings(ctx, suid, scene, params, req.GetManualCount())
		if err != nil {
			return nil, err
		}
		response = &papi.PhotoCreateShootingsResponse{
			Photos:         result.photos,
			IsGift:         result.isGift,
			CommonResponse: viber.NewCommonResponse(result.updated),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *PhotoServer) CreateStoryEpisodeShootings(
	ctx context.Context,
	req *papi.PhotoCreateStoryEpisodeShootingsRequest,
) (*papi.PhotoCreateStoryEpisodeShootingsResponse, error) {
	if len(req.GetCreateShootingParams()) == 0 {
		return nil, viber.InvalidArgumentf("at least one createShootingParam is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoCreateStoryEpisodeShootingsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := s.CheckStoryEpisodeShooting(ctx, &papi.PhotoCheckStoryEpisodeShootingRequest{
			StoryPartId: req.GetStoryPartId(),
			Chapter:     req.GetChapter(),
			Route:       req.GetRoute(),
			Episode:     req.GetEpisode(),
		}); err != nil {
			return nil, err
		}
		part, err := opmaster.FindStoryPart(ctx, req.GetStoryPartId())
		if err != nil {
			return nil, err
		}
		storyID, err := resolvePhotoStoryEpisode(ctx, req.GetStoryPartId(), req.GetChapter(), req.GetRoute(), req.GetEpisode())
		if err != nil {
			return nil, err
		}
		candidates, err := photoStoryCharacters(ctx, storyID)
		if err != nil {
			return nil, err
		}
		scene := photoShootingScene{
			placeName:             fmt.Sprintf("%s %d章 %d話", part.GetName(), req.GetChapter(), req.GetEpisode()),
			eventName:             storyPartEventName(part.GetName()),
			candidateCharacterIDs: candidates,
			storyID:               storyID,
			storyShooting:         true,
		}
		result, err := createUserPhotoShootings(
			ctx, suid, scene, commonPhotoParams(req.GetCreateShootingParams()), req.GetManualCount(),
		)
		if err != nil {
			return nil, err
		}
		response = &papi.PhotoCreateStoryEpisodeShootingsResponse{
			Photos: result.photos, IsGift: result.isGift, CommonResponse: viber.NewCommonResponse(result.updated),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *PhotoServer) CreateCardStoryShootings(
	ctx context.Context,
	req *papi.PhotoCreateCardStoryShootingsRequest,
) (*papi.PhotoCreateCardStoryShootingsResponse, error) {
	if len(req.GetCreateShootingParams()) == 0 {
		return nil, viber.InvalidArgumentf("at least one createShootingParam is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoCreateCardStoryShootingsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := s.CheckCardStoryShooting(ctx, &papi.PhotoCheckCardStoryShootingRequest{
			CardId: req.GetCardId(), Number: req.GetNumber(),
		}); err != nil {
			return nil, err
		}
		card, err := opmaster.FindCard(ctx, req.GetCardId())
		if err != nil {
			return nil, err
		}
		storyID := card.GetStories()[req.GetNumber()-1].GetStoryId()
		candidates, err := photoStoryCharacters(ctx, storyID)
		if err != nil {
			return nil, err
		}
		candidates = appendUniquePhotoString(candidates, card.GetCharacterId())
		scene := photoShootingScene{
			placeName:             fmt.Sprintf("%s %d話", card.GetName(), req.GetNumber()),
			eventName:             "アイドル編",
			candidateCharacterIDs: candidates,
			storyID:               storyID,
			storyShooting:         true,
		}
		result, err := createUserPhotoShootings(
			ctx, suid, scene, commonPhotoParams(req.GetCreateShootingParams()), req.GetManualCount(),
		)
		if err != nil {
			return nil, err
		}
		response = &papi.PhotoCreateCardStoryShootingsResponse{
			Photos: result.photos, IsGift: result.isGift, CommonResponse: viber.NewCommonResponse(result.updated),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *PhotoServer) CreateExpressionShootings(
	ctx context.Context,
	req *papi.PhotoCreateExpressionShootingsRequest,
) (*papi.PhotoCreateExpressionShootingsResponse, error) {
	if len(req.GetCreateExpressionShootingParams()) == 0 {
		return nil, viber.InvalidArgumentf("at least one createExpressionShootingParam is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoCreateExpressionShootingsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := s.CheckExpressionShooting(ctx, &papi.PhotoCheckExpressionShootingRequest{
			PhotoExpressionId: req.GetPhotoExpressionId(),
			CharacterId:       req.GetCharacterId(),
			CostumeId:         req.GetCostumeId(),
			HairId:            req.GetHairId(),
			CharacterIds:      req.GetCharacterIds(),
			CostumeIds:        req.GetCostumeIds(),
			HairIds:           req.GetHairIds(),
		}); err != nil {
			return nil, err
		}
		master, err := opmaster.FindPhotoExpression(ctx, req.GetPhotoExpressionId())
		if err != nil {
			return nil, err
		}
		stage, err := opmaster.FindPhotoStage(ctx, master.GetStageId())
		if err != nil {
			return nil, err
		}
		positions, err := opmaster.FindPhotoExpressionStagePositions(ctx, master.GetStageId())
		if err != nil {
			return nil, err
		}
		validPositions := make(map[int32]struct{}, len(positions))
		for _, position := range positions {
			validPositions[position.GetNumber()] = struct{}{}
		}
		candidateIDs := req.GetCharacterIds()
		if len(candidateIDs) == 0 {
			candidateIDs = []string{req.GetCharacterId()}
		}
		params := make([]photoCreateParam, 0, len(req.GetCreateExpressionShootingParams()))
		specifiedPoseCounts := make(map[string]int64)
		specifiedCharacterCounts := make(map[string]int64)
		for _, param := range req.GetCreateExpressionShootingParams() {
			if param == nil {
				return nil, viber.InvalidArgumentf("createExpressionShootingParam is required")
			}
			if _, exists := validPositions[param.GetStagePositionNumber()]; !exists {
				return nil, viber.InvalidArgumentf("unknown expression stage position")
			}
			if err := validateExpressionPhotoParam(ctx, suid, param); err != nil {
				return nil, err
			}
			if param.GetPhotoPoseId() != "" {
				specifiedPoseCounts[param.GetPhotoPoseId()]++
			}
			if param.GetMainCharacterId() != "" {
				specifiedCharacterCounts[param.GetMainCharacterId()]++
			}
			params = append(params, photoCreateParam{
				imageID: param.GetPhotoImageId(), itemID: param.GetItemId(), focusCharacterID: param.GetMainCharacterId(),
			})
		}
		scene := photoShootingScene{
			placeName:                stage.GetName(),
			eventName:                master.GetName(),
			candidateCharacterIDs:    slices.Clone(candidateIDs),
			expressionShooting:       true,
			specifiedPoseCounts:      specifiedPoseCounts,
			specifiedCharacterCounts: specifiedCharacterCounts,
		}
		result, err := createUserPhotoShootings(ctx, suid, scene, params, req.GetManualCount())
		if err != nil {
			return nil, err
		}
		response = &papi.PhotoCreateExpressionShootingsResponse{
			Photos: result.photos, IsGift: result.isGift, CommonResponse: viber.NewCommonResponse(result.updated),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *PhotoServer) CreateExcursionShootings(
	ctx context.Context,
	req *papi.PhotoCreateExcursionShootingsRequest,
) (*papi.PhotoCreateExcursionShootingsResponse, error) {
	if len(req.GetCreateExcursionShootingParams()) == 0 {
		return nil, viber.InvalidArgumentf("at least one createExcursionShootingParam is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoCreateExcursionShootingsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := s.CheckExcursionShooting(ctx, &papi.PhotoCheckExcursionShootingRequest{
			ExcursionPlaceId: req.GetExcursionPlaceId(), CharacterId: req.GetCharacterId(),
			CostumeId: req.GetCostumeId(), HairId: req.GetHairId(),
		}); err != nil {
			return nil, err
		}
		place, err := opmaster.FindExcursionPlace(ctx, req.GetExcursionPlaceId())
		if err != nil {
			return nil, err
		}
		params := make([]photoCreateParam, 0, len(req.GetCreateExcursionShootingParams()))
		for _, param := range req.GetCreateExcursionShootingParams() {
			if param == nil {
				return nil, viber.InvalidArgumentf("createExcursionShootingParam is required")
			}
			params = append(params, photoCreateParam{
				imageID: param.GetPhotoImageId(), itemID: param.GetItemId(), focusCharacterID: req.GetCharacterId(),
			})
		}
		scene := photoShootingScene{
			placeName: place.GetName(), eventName: place.GetName(), candidateCharacterIDs: []string{req.GetCharacterId()},
		}
		result, err := createUserPhotoShootings(ctx, suid, scene, params, req.GetManualCount())
		if err != nil {
			return nil, err
		}
		response = &papi.PhotoCreateExcursionShootingsResponse{
			Photos: result.photos, IsGift: result.isGift, CommonResponse: viber.NewCommonResponse(result.updated),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func createUserPhotoShootings(
	ctx context.Context,
	suid string,
	scene photoShootingScene,
	params []photoCreateParam,
	manualCount int32,
) (*photoCreationResult, error) {
	if len(params) == 0 || len(params) > reservedPhotoImageIDCount {
		return nil, viber.InvalidArgumentf("shooting count must be between 1 and %d", reservedPhotoImageIDCount)
	}
	if manualCount < 0 {
		return nil, viber.InvalidArgumentf("manualCount must not be negative")
	}
	setting, err := photoSetting(ctx)
	if err != nil {
		return nil, err
	}
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, err
	}
	photosBefore, err := opuser.FindAllUserPhotos(ctx, suid)
	if err != nil {
		return nil, err
	}
	capacity, err := viber.PhotoCapacityLimit(setting, user)
	if err != nil {
		return nil, err
	}
	availableCapacity := max(int64(0), capacity-int64(len(photosBefore)))
	ownedPhotoCount := min(len(params), int(availableCapacity))

	reserved := make(map[string]struct{}, len(user.GetNextPhotoImageIds()))
	for _, id := range user.GetNextPhotoImageIds() {
		reserved[id] = struct{}{}
	}
	usedImageIDs := make(map[string]struct{}, len(params))
	filmAmounts := make(map[string]int64)
	filmOrder := make([]string, 0)
	films := make(map[string]*pmaster.Item)
	for index := range params {
		param := &params[index]
		if param.imageID == "" || param.itemID == "" {
			return nil, viber.InvalidArgumentf("photoImageId and itemId are required")
		}
		if _, exists := reserved[param.imageID]; !exists {
			return nil, status.Errorf(codes.FailedPrecondition, "photo image ID %q is not reserved for this user", param.imageID)
		}
		if _, duplicate := usedImageIDs[param.imageID]; duplicate {
			return nil, viber.InvalidArgumentf("photoImageId values must be unique")
		}
		usedImageIDs[param.imageID] = struct{}{}
		if _, exists := films[param.itemID]; !exists {
			item, err := opmaster.FindItem(ctx, param.itemID)
			if err != nil {
				return nil, err
			}
			if item.GetType() != penum.ItemType_ItemType_PhotoShooting &&
				item.GetType() != penum.ItemType_ItemType_PhotoShootingLevelChange {
				return nil, status.Errorf(codes.FailedPrecondition, "item %q is not a shooting film", param.itemID)
			}
			if scene.storyShooting && item.GetIsPhotoShootingCharacterOnly() {
				return nil, status.Error(codes.FailedPrecondition, "character-only film cannot be used for story shooting")
			}
			films[param.itemID] = item
			filmOrder = append(filmOrder, param.itemID)
		}
		filmAmounts[param.itemID]++
	}

	updated := &papi.UpdatedData{}
	for _, itemID := range filmOrder {
		if err := viber.ConsumeExchangeResource(
			ctx, suid, penum.ResourceType_ResourceType_Item, itemID, filmAmounts[itemID], updated,
		); err != nil {
			return nil, err
		}
	}
	generator, err := viber.NewPhotoGenerator(ctx, setting)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	shootingTime := now.UnixMilli()
	date := now.In(time.FixedZone("JST", 9*60*60)).Format("1/2")
	created := make([]*ptransaction.UserPhoto, 0, len(params))
	reliabilityIncreases := make(map[string]int32)
	var highestPhotoLevel int64
	for index := range params {
		param := &params[index]
		focusID, err := resolvePhotoFocusCharacter(ctx, suid, param.focusCharacterID, scene.candidateCharacterIDs)
		if err != nil {
			return nil, err
		}
		character, err := opmaster.FindCharacter(ctx, focusID)
		if err != nil {
			return nil, err
		}
		traits, err := generator.Generate(ctx, films[param.itemID], focusID, user.GetManagerLevel())
		if err != nil {
			return nil, err
		}
		photo := &ptransaction.UserPhoto{
			PhotoId:          uuid.NewString(),
			AssetId:          param.imageID,
			ImageType:        penum.PhotoImageType_PhotoImageType_User,
			Name:             strings.TrimSpace(character.GetName()) + " " + date,
			Rarity:           traits.Rarity,
			PlaceName:        scene.placeName,
			EventName:        scene.eventName,
			Locked:           false,
			Level:            traits.Level,
			Rerollable:       true,
			Abilities:        traits.Abilities,
			ShootingTime:     shootingTime,
			FocusCharacterId: focusID,
		}
		if traits.Dedicated {
			photo.CharacterIds = []string{focusID}
		}
		created = append(created, photo)
		if int64(photo.GetLevel()) > highestPhotoLevel {
			highestPhotoLevel = int64(photo.GetLevel())
		}
		if character.GetType() != penum.CharacterType_CharacterType_Npc {
			reliabilityIncreases[focusID]++
		}
	}
	ownedPhotos := created[:ownedPhotoCount]
	overflowPhotos := created[ownedPhotoCount:]
	if err := opuser.UpsertManyUserPhotos(ctx, suid, ownedPhotos); err != nil {
		return nil, err
	}
	updated.Photos = append(updated.Photos, ownedPhotos...)
	responsePhotos := make([]*ptransaction.UserPhoto, 0, len(created))
	responsePhotos = append(responsePhotos, ownedPhotos...)
	for _, photo := range overflowPhotos {
		if err := opuser.UpsertUserPhotoGift(ctx, suid, photo); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserGift(ctx, suid, &ptransaction.UserGift{
			GiftId:       uuid.NewString(),
			ResourceType: penum.ResourceType_ResourceType_PhotoGift,
			ResourceId:   photo.GetPhotoId(),
			Amount:       1,
			Message:      viber.PhotoCapacityGiftMessage,
			PostedTime:   shootingTime,
			LimitTime:    shootingTime + photoGiftRetentionMillis,
			PhotoGift:    &ptransaction.PhotoGift{Name: photo.GetName()},
		}); err != nil {
			return nil, err
		}
		responsePhoto := proto.Clone(photo).(*ptransaction.UserPhoto)
		for _, ability := range responsePhoto.GetAbilities() {
			ability.IsAvailable = false
		}
		responsePhotos = append(responsePhotos, responsePhoto)
	}
	if len(overflowPhotos) > 0 {
		if err := viber.SetGiftNotification(ctx, suid, true, updated); err != nil {
			return nil, err
		}
	}

	for characterID, increase := range reliabilityIncreases {
		character, err := opuser.FindUserCharacter(ctx, suid, characterID)
		if err != nil {
			return nil, err
		}
		if increase <= 0 || character.GetReliability() > math.MaxInt32-increase {
			return nil, fmt.Errorf("character reliability overflows")
		}
		character.Reliability += increase
		if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
			return nil, err
		}
		updated.Characters = append(updated.Characters, character)
		characterMissions, err := viber.AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_CharacterPhotoShootCount, int64(increase), characterID,
		)
		if err != nil {
			return nil, err
		}
		reliabilityMissions, err := viber.ReachUserMissions(
			ctx, suid, penum.MissionType_MissionType_ReliabilityAchievement, int64(character.GetReliability()), characterID,
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = viber.AppendUniquePhotoMissions(updated.Missions, characterMissions, reliabilityMissions)
	}

	total, err := opuser.FindUserTotalCount(ctx, suid)
	if err != nil {
		return nil, err
	}
	if total.GetPhotoShootCount() > math.MaxInt64-int64(len(created)) {
		return nil, fmt.Errorf("photo shoot count overflows")
	}
	total.PhotoShootCount += int64(len(created))
	if err := opuser.UpsertUserTotalCount(ctx, suid, total); err != nil {
		return nil, err
	}
	updated.TotalCount = total
	count := int64(len(created))
	shootMissions, err := viber.AdvanceUserMissions(ctx, suid, penum.MissionType_MissionType_PhotoShootCount, count)
	if err != nil {
		return nil, err
	}
	totalMissions, err := viber.AdvanceUserMissions(ctx, suid, penum.MissionType_MissionType_PhotoShootTotalCount, count)
	if err != nil {
		return nil, err
	}
	updated.Missions = viber.AppendUniquePhotoMissions(updated.Missions, shootMissions, totalMissions)
	levelMissions, err := viber.ReachUserMissions(
		ctx, suid, penum.MissionType_MissionType_PhotoLevelAchievement, highestPhotoLevel,
	)
	if err != nil {
		return nil, err
	}
	updated.Missions = viber.AppendUniquePhotoMissions(updated.Missions, levelMissions)
	if scene.storyShooting {
		missions, err := viber.AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_PhotoShootingStoryShootTotalCount, count,
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = viber.AppendUniquePhotoMissions(updated.Missions, missions)
	}
	if scene.expressionShooting {
		missions, err := viber.AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_PhotoExpressionShootingCount, count,
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = viber.AppendUniquePhotoMissions(updated.Missions, missions)
		if len(scene.specifiedPoseCounts) > 0 {
			poseTargetIDs := photoMissionTargetIDs(scene.specifiedPoseCounts)
			poseMissions, err := viber.AdvanceUserMissionsWithAmount(
				ctx, suid, penum.MissionType_MissionType_PhotoExpressionShootingSpecifiedPoseShootCount,
				func(master *pmaster.Mission) int64 {
					return photoMissionTargetAmount(master, scene.specifiedPoseCounts, count)
				},
				poseTargetIDs...,
			)
			if err != nil {
				return nil, err
			}
			updated.Missions = viber.AppendUniquePhotoMissions(updated.Missions, poseMissions)
		}
		if len(scene.specifiedCharacterCounts) > 0 {
			characterTargetIDs := photoMissionTargetIDs(scene.specifiedCharacterCounts)
			characterMissions, err := viber.AdvanceUserMissionsWithAmount(
				ctx, suid, penum.MissionType_MissionType_PhotoExpressionShootingSpecifiedCharacterShootCount,
				func(master *pmaster.Mission) int64 {
					return photoMissionTargetAmount(master, scene.specifiedCharacterCounts, count)
				},
				characterTargetIDs...,
			)
			if err != nil {
				return nil, err
			}
			updated.Missions = viber.AppendUniquePhotoMissions(updated.Missions, characterMissions)
		}
	}
	if scene.musicID != "" {
		missions, err := viber.AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_PhotoSpecifiedMusicShootingCount, count, scene.musicID,
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = viber.AppendUniquePhotoMissions(updated.Missions, missions)
	}
	if scene.storyID != "" {
		story, err := opuser.FindUserStory(ctx, suid, scene.storyID)
		if err != nil {
			return nil, err
		}
		if !story.GetIsPhotoShot() {
			story.IsPhotoShot = true
			if err := opuser.UpsertUserStory(ctx, suid, story); err != nil {
				return nil, err
			}
			updated.Stories = append(updated.Stories, story)
		}
	}

	remainingIDs := make([]string, 0, reservedPhotoImageIDCount)
	for _, id := range user.GetNextPhotoImageIds() {
		if _, used := usedImageIDs[id]; !used {
			remainingIDs = append(remainingIDs, id)
		}
	}
	for len(remainingIDs) < reservedPhotoImageIDCount {
		candidate := uuid.NewString()
		if _, exists := reserved[candidate]; exists {
			continue
		}
		reserved[candidate] = struct{}{}
		remainingIDs = append(remainingIDs, candidate)
	}
	user.NextPhotoImageIds = remainingIDs[:reservedPhotoImageIDCount]
	if err := opuser.UpsertUser(ctx, suid, user); err != nil {
		return nil, err
	}
	updated.User = user
	dedupePhotoUpdatedItems(updated)
	mcontext.GetLogger(ctx).Debugw(
		"photo shooting statistics",
		"manual_count", manualCount,
		"created_count", len(created),
		"gift_count", len(overflowPhotos),
	)
	return &photoCreationResult{
		photos: responsePhotos, isGift: len(overflowPhotos) > 0, updated: updated,
	}, nil
}

func photoMissionTargetIDs(amounts map[string]int64) []string {
	ids := make([]string, 0, len(amounts))
	for id := range amounts {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func photoMissionTargetAmount(master *pmaster.Mission, amounts map[string]int64, untargetedAmount int64) int64 {
	if len(master.GetTargetIds()) == 0 {
		return untargetedAmount
	}
	var amount int64
	for _, id := range master.GetTargetIds() {
		amount += amounts[id]
	}
	return amount
}

func normalPhotoScene(ctx context.Context, req *papi.PhotoCreateShootingsRequest) (photoShootingScene, error) {
	scene := photoShootingScene{candidateCharacterIDs: slices.Clone(req.GetCharacterIds())}
	switch req.GetActionType() {
	case penum.PhotoShootingActionType_PhotoShootingActionType_Quest:
		stage, err := opmaster.FindPhotoQuestStage(ctx, req.GetPhotoStageId())
		if err != nil {
			return scene, err
		}
		scene.placeName = stage.GetName()
		scene.eventName = "ライブ"
		scene.musicID = req.GetPhotoMusicId()
	case penum.PhotoShootingActionType_PhotoShootingActionType_ActivityPromotion,
		penum.PhotoShootingActionType_PhotoShootingActionType_ActivityRefresh:
		activity, err := opmaster.FindPhotoActivity(ctx, req.GetPhotoActivityId())
		if err != nil {
			return scene, err
		}
		scene.placeName = activity.GetName()
		scene.eventName = activity.GetName()
	default:
		return scene, viber.InvalidArgumentf("unsupported photo shooting action type")
	}
	return scene, nil
}

func commonPhotoParams(params []*papi.PhotoCreateShootingParam) []photoCreateParam {
	result := make([]photoCreateParam, 0, len(params))
	for _, param := range params {
		if param == nil {
			result = append(result, photoCreateParam{})
			continue
		}
		result = append(result, photoCreateParam{
			imageID: param.GetPhotoImageId(), itemID: param.GetItemId(), focusCharacterID: param.GetMainCharacterId(),
		})
	}
	return result
}

func resolvePhotoFocusCharacter(
	ctx context.Context,
	suid string,
	requested string,
	candidates []string,
) (string, error) {
	if requested != "" {
		master, err := opmaster.FindCharacter(ctx, requested)
		if err != nil {
			return "", photoMasterLookupError("character", requested, err)
		}
		if master.GetType() == penum.CharacterType_CharacterType_Npc {
			return requested, nil
		}
		if _, err := opuser.FindUserCharacter(ctx, suid, requested); err != nil {
			return "", photoOwnershipError("character", requested, err)
		}
		return requested, nil
	}
	ownedCandidates := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" || slices.Contains(ownedCandidates, candidate) {
			continue
		}
		if _, err := opuser.FindUserCharacter(ctx, suid, candidate); err == nil {
			ownedCandidates = append(ownedCandidates, candidate)
		}
	}
	if len(ownedCandidates) == 0 {
		characters, err := opuser.FindAllUserCharacters(ctx, suid)
		if err != nil {
			return "", err
		}
		for _, character := range characters {
			if character.GetCharacterId() != "" {
				ownedCandidates = append(ownedCandidates, character.GetCharacterId())
			}
		}
	}
	if len(ownedCandidates) == 0 {
		return "", status.Error(codes.FailedPrecondition, "no photographed character can be determined")
	}
	return ownedCandidates[rand.IntN(len(ownedCandidates))], nil
}

func photoStoryCharacters(ctx context.Context, storyID string) ([]string, error) {
	story, err := opmaster.FindStory(ctx, storyID)
	if err != nil {
		return nil, err
	}
	result := slices.Clone(story.GetAdditionalDisplayCharacterIds())
	setting, err := photoSetting(ctx)
	if err != nil {
		return nil, err
	}
	if setting.GetStoryPhotoShootingDefaultCharacterId() != "" {
		result = appendUniquePhotoString(result, setting.GetStoryPhotoShootingDefaultCharacterId())
	}
	return result, nil
}

func storyPartEventName(name string) string {
	if strings.HasSuffix(name, "編") {
		return name
	}
	return name + "編"
}

func appendUniquePhotoString(values []string, value string) []string {
	if value != "" && !slices.Contains(values, value) {
		values = append(values, value)
	}
	return values
}

func validateExpressionPhotoParam(
	ctx context.Context,
	suid string,
	param *papi.PhotoCreateExpressionShootingParam,
) error {
	if param.GetPhotoImageId() == "" || param.GetItemId() == "" || param.GetMainCharacterId() == "" {
		return viber.InvalidArgumentf("photoImageId, itemId, and mainCharacterId are required")
	}
	if len(param.GetCharacterIds()) > 0 {
		if len(param.GetPhotoPoseIds()) != len(param.GetCharacterIds()) ||
			len(param.GetPhotoFacialIds()) != len(param.GetCharacterIds()) ||
			len(param.GetPhotoLookingTypes()) != len(param.GetCharacterIds()) {
			return viber.InvalidArgumentf("expression character option lists must have equal lengths")
		}
	}
	poseIDs := append(slices.Clone(param.GetPhotoPoseIds()), param.GetPhotoPoseId())
	for _, poseID := range poseIDs {
		if poseID == "" {
			continue
		}
		if _, err := opmaster.FindPhotoPose(ctx, poseID); err != nil {
			return photoMasterLookupError("photo pose", poseID, err)
		}
		if _, err := opuser.FindUserPhotoPose(ctx, suid, poseID); err != nil {
			return photoOwnershipError("photo pose", poseID, err)
		}
	}
	facialIDs := append(slices.Clone(param.GetPhotoFacialIds()), param.GetPhotoFacialId())
	for _, facialID := range facialIDs {
		if facialID == "" {
			continue
		}
		if _, err := opmaster.FindPhotoFacial(ctx, facialID); err != nil {
			return photoMasterLookupError("photo facial", facialID, err)
		}
	}
	if !slices.Contains(param.GetCharacterIds(), param.GetMainCharacterId()) && len(param.GetCharacterIds()) > 0 {
		return viber.FailedPreconditionf("main character is absent from expression characters")
	}
	return nil
}
