package viber

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
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	ReservedPhotoImageIDCount = 20
	photoGiftRetentionMillis  = int64((14 * 24 * time.Hour) / time.Millisecond)
)

type PhotoCreateParam struct {
	ImageID          string
	ItemID           string
	FocusCharacterID string
}

type PhotoShootingScene struct {
	PlaceName                   string
	EventName                   string
	PhotoNamePrefix             string
	CandidateCharacterIDs       []string
	StoryID                     string
	StoryShooting               bool
	ExpressionShooting          bool
	PanoramaShooting            bool
	ShowcaseShooting            bool
	ReliabilityOncePerCharacter bool
	MusicID                     string
	SpecifiedPoseCounts         map[string]int64
	SpecifiedCharacterCounts    map[string]int64
}

type PhotoCreationResult struct {
	Photos  []*ptransaction.UserPhoto
	IsGift  bool
	Updated *papi.UpdatedData
}

func CreateUserPhotoShootings(
	ctx context.Context,
	suid string,
	scene PhotoShootingScene,
	params []PhotoCreateParam,
	manualCount int32,
) (*PhotoCreationResult, error) {
	if len(params) == 0 || len(params) > ReservedPhotoImageIDCount {
		return nil, InvalidArgumentf("shooting count must be between 1 and %d", ReservedPhotoImageIDCount)
	}
	if manualCount < 0 {
		return nil, InvalidArgumentf("manualCount must not be negative")
	}
	setting, err := PhotoSetting(ctx)
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
	capacity, err := PhotoCapacityLimit(setting, user)
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
		if param.ImageID == "" || param.ItemID == "" {
			return nil, InvalidArgumentf("photoImageId and itemId are required")
		}
		if _, exists := reserved[param.ImageID]; !exists {
			return nil, status.Errorf(codes.FailedPrecondition, "photo image ID %q is not reserved for this user", param.ImageID)
		}
		if _, duplicate := usedImageIDs[param.ImageID]; duplicate {
			return nil, InvalidArgumentf("photoImageId values must be unique")
		}
		usedImageIDs[param.ImageID] = struct{}{}
		if _, exists := films[param.ItemID]; !exists {
			item, err := opmaster.FindItem(ctx, param.ItemID)
			if err != nil {
				return nil, err
			}
			if item.GetType() != penum.ItemType_ItemType_PhotoShooting &&
				item.GetType() != penum.ItemType_ItemType_PhotoShootingLevelChange {
				return nil, status.Errorf(codes.FailedPrecondition, "item %q is not a shooting film", param.ItemID)
			}
			if scene.StoryShooting && item.GetIsPhotoShootingCharacterOnly() {
				return nil, status.Error(codes.FailedPrecondition, "character-only film cannot be used for story shooting")
			}
			films[param.ItemID] = item
			filmOrder = append(filmOrder, param.ItemID)
		}
		filmAmounts[param.ItemID]++
	}

	updated := &papi.UpdatedData{}
	for _, itemID := range filmOrder {
		if err := ConsumeExchangeResource(
			ctx, suid, penum.ResourceType_ResourceType_Item, itemID, filmAmounts[itemID], updated,
		); err != nil {
			return nil, err
		}
	}
	generator, err := NewPhotoGenerator(ctx, setting)
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
		focusID, err := resolvePhotoFocusCharacter(ctx, suid, param.FocusCharacterID, scene.CandidateCharacterIDs)
		if err != nil {
			return nil, err
		}
		character, err := opmaster.FindCharacter(ctx, focusID)
		if err != nil {
			return nil, err
		}
		traits, err := generator.Generate(ctx, films[param.ItemID], focusID, user.GetManagerLevel())
		if err != nil {
			return nil, err
		}
		photoName := strings.TrimSpace(character.GetName()) + " " + date
		if scene.PhotoNamePrefix != "" {
			photoName = strings.TrimSpace(scene.PhotoNamePrefix) + " " + date
		}
		photo := &ptransaction.UserPhoto{
			PhotoId:          uuid.NewString(),
			AssetId:          param.ImageID,
			ImageType:        penum.PhotoImageType_PhotoImageType_User,
			Name:             photoName,
			Rarity:           traits.Rarity,
			PlaceName:        scene.PlaceName,
			EventName:        scene.EventName,
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
			if scene.ReliabilityOncePerCharacter {
				reliabilityIncreases[focusID] = 1
			} else {
				reliabilityIncreases[focusID]++
			}
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
			Message:      PhotoCapacityGiftMessage,
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
		if err := SetGiftNotification(ctx, suid, true, updated); err != nil {
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
		characterMissions, err := AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_CharacterPhotoShootCount, int64(increase), characterID,
		)
		if err != nil {
			return nil, err
		}
		reliabilityMissions, err := ReachUserMissions(
			ctx, suid, penum.MissionType_MissionType_ReliabilityAchievement, int64(character.GetReliability()), characterID,
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = AppendUniquePhotoMissions(updated.Missions, characterMissions, reliabilityMissions)
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
	shootMissions, err := AdvanceUserMissions(ctx, suid, penum.MissionType_MissionType_PhotoShootCount, count)
	if err != nil {
		return nil, err
	}
	totalMissions, err := AdvanceUserMissions(ctx, suid, penum.MissionType_MissionType_PhotoShootTotalCount, count)
	if err != nil {
		return nil, err
	}
	updated.Missions = AppendUniquePhotoMissions(updated.Missions, shootMissions, totalMissions)
	levelMissions, err := ReachUserMissions(
		ctx, suid, penum.MissionType_MissionType_PhotoLevelAchievement, highestPhotoLevel,
	)
	if err != nil {
		return nil, err
	}
	updated.Missions = AppendUniquePhotoMissions(updated.Missions, levelMissions)
	if scene.StoryShooting {
		missions, err := AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_PhotoShootingStoryShootTotalCount, count,
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = AppendUniquePhotoMissions(updated.Missions, missions)
	}
	if scene.ExpressionShooting {
		missions, err := AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_PhotoExpressionShootingCount, count,
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = AppendUniquePhotoMissions(updated.Missions, missions)
		if len(scene.SpecifiedPoseCounts) > 0 {
			poseTargetIDs := photoMissionTargetIDs(scene.SpecifiedPoseCounts)
			poseMissions, err := AdvanceUserMissionsWithAmount(
				ctx, suid, penum.MissionType_MissionType_PhotoExpressionShootingSpecifiedPoseShootCount,
				func(master *pmaster.Mission) int64 {
					return photoMissionTargetAmount(master, scene.SpecifiedPoseCounts, count)
				},
				poseTargetIDs...,
			)
			if err != nil {
				return nil, err
			}
			updated.Missions = AppendUniquePhotoMissions(updated.Missions, poseMissions)
		}
		if len(scene.SpecifiedCharacterCounts) > 0 {
			characterTargetIDs := photoMissionTargetIDs(scene.SpecifiedCharacterCounts)
			characterMissions, err := AdvanceUserMissionsWithAmount(
				ctx, suid, penum.MissionType_MissionType_PhotoExpressionShootingSpecifiedCharacterShootCount,
				func(master *pmaster.Mission) int64 {
					return photoMissionTargetAmount(master, scene.SpecifiedCharacterCounts, count)
				},
				characterTargetIDs...,
			)
			if err != nil {
				return nil, err
			}
			updated.Missions = AppendUniquePhotoMissions(updated.Missions, characterMissions)
		}
	}
	if scene.PanoramaShooting {
		missions, err := AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_PhotoPanoramaShootingCount, count,
		)
		if err != nil {
			return nil, err
		}
		totalMissions, err := AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_PhotoPanoramaShootingTotalCount, count,
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = AppendUniquePhotoMissions(updated.Missions, missions, totalMissions)
	}
	if scene.ShowcaseShooting {
		missions, err := AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_ShowcasePhotoShootingTotalCount, count,
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = AppendUniquePhotoMissions(updated.Missions, missions)
	}
	if scene.MusicID != "" {
		missions, err := AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_PhotoSpecifiedMusicShootingCount, count, scene.MusicID,
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = AppendUniquePhotoMissions(updated.Missions, missions)
	}
	if scene.StoryID != "" {
		story, err := opuser.FindUserStory(ctx, suid, scene.StoryID)
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

	remainingIDs := make([]string, 0, ReservedPhotoImageIDCount)
	for _, id := range user.GetNextPhotoImageIds() {
		if _, used := usedImageIDs[id]; !used {
			remainingIDs = append(remainingIDs, id)
		}
	}
	for len(remainingIDs) < ReservedPhotoImageIDCount {
		candidate := uuid.NewString()
		if _, exists := reserved[candidate]; exists {
			continue
		}
		reserved[candidate] = struct{}{}
		remainingIDs = append(remainingIDs, candidate)
	}
	user.NextPhotoImageIds = remainingIDs[:ReservedPhotoImageIDCount]
	if err := opuser.UpsertUser(ctx, suid, user); err != nil {
		return nil, err
	}
	updated.User = user
	DedupePhotoUpdatedItems(updated)
	mcontext.GetLogger(ctx).Debugw(
		"photo shooting statistics",
		"manual_count", manualCount,
		"created_count", len(created),
		"gift_count", len(overflowPhotos),
	)
	return &PhotoCreationResult{
		Photos: responsePhotos, IsGift: len(overflowPhotos) > 0, Updated: updated,
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

func CommonPhotoParams(params []*papi.PhotoCreateShootingParam) []PhotoCreateParam {
	result := make([]PhotoCreateParam, 0, len(params))
	for _, param := range params {
		if param == nil {
			result = append(result, PhotoCreateParam{})
			continue
		}
		result = append(result, PhotoCreateParam{
			ImageID: param.GetPhotoImageId(), ItemID: param.GetItemId(), FocusCharacterID: param.GetMainCharacterId(),
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
			return "", PhotoMasterLookupError("character", requested, err)
		}
		if master.GetType() == penum.CharacterType_CharacterType_Npc {
			return requested, nil
		}
		if _, err := opuser.FindUserCharacter(ctx, suid, requested); err != nil {
			return "", PhotoOwnershipError("character", requested, err)
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
