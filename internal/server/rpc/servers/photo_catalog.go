package servers

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strings"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	photoFirebaseBucket = "udc-idolypride"
	photoObjectPrefix   = "Photo"
)

type PhotoServer struct {
	papi.UnimplementedPhotoServer
}

func (s *PhotoServer) GetImageURL(
	ctx context.Context,
	req *papi.PhotoGetImageUrlRequest,
) (*papi.PhotoGetImageUrlResponse, error) {
	if req.GetType() != penum.PhotoImageRequestType_PhotoImageRequestType_Original &&
		req.GetType() != penum.PhotoImageRequestType_PhotoImageRequestType_Thumbnail &&
		req.GetType() != penum.PhotoImageRequestType_PhotoImageRequestType_Both {
		return nil, viber.InvalidArgumentf("unsupported photo image request type")
	}

	suid := viber.GetSuid(ctx).String()
	public, err := opuser.FindUserPublic(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}

	names := make([]string, len(req.GetAssetIds()))
	for index, assetID := range req.GetAssetIds() {
		if !isPhotoAssetID(assetID) {
			return nil, viber.InvalidArgumentf("invalid photo assetId %q", assetID)
		}
		names[index] = photoObjectPrefix + "/" + public.GetPublicUserId() + "/" + assetID
	}
	metadata, err := opuser.FindFinalizedUserPhotoImageMetadataByNames(ctx, suid, names)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	metadataByName := make(map[string]*umodel.UserPhotoImageMetadata, len(metadata))
	for _, entry := range metadata {
		if entry.UploadState == umodel.UserPhotoImageUploadFinalized &&
			entry.PublicUserId == public.GetPublicUserId() {
			metadataByName[entry.Name] = entry
		}
	}

	urls := make([]*papi.PhotoImageUrl, 0, len(names))
	for index, name := range names {
		downloadURL := photoFirebaseDownloadURL(name, metadataByName[name])
		entry := &papi.PhotoImageUrl{AssetId: req.AssetIds[index]}
		switch req.GetType() {
		case penum.PhotoImageRequestType_PhotoImageRequestType_Original:
			entry.OrigUrl = downloadURL
		case penum.PhotoImageRequestType_PhotoImageRequestType_Thumbnail:
			entry.TmbUrl = downloadURL
		case penum.PhotoImageRequestType_PhotoImageRequestType_Both:
			entry.OrigUrl = downloadURL
			entry.TmbUrl = downloadURL
		}
		urls = append(urls, entry)
	}
	return &papi.PhotoGetImageUrlResponse{UrlList: urls}, nil
}

func (s *PhotoServer) CheckShooting(
	ctx context.Context,
	req *papi.PhotoCheckShootingRequest,
) (*papi.PhotoCheckShootingResponse, error) {
	if err := validatePhotoStyleLists(req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds()); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()

	switch req.GetActionType() {
	case penum.PhotoShootingActionType_PhotoShootingActionType_ActivityPromotion,
		penum.PhotoShootingActionType_PhotoShootingActionType_ActivityRefresh:
		if req.GetPhotoActivityId() == "" {
			return nil, viber.InvalidArgumentf("photoActivityId is required")
		}
		activity, err := opmaster.FindPhotoActivity(ctx, req.GetPhotoActivityId())
		if err != nil {
			return nil, photoMasterLookupError("activity", req.GetPhotoActivityId(), err)
		}
		if activity.GetType() != req.GetActionType() {
			return nil, viber.InvalidArgumentf("photo activity %q has a different action type", req.GetPhotoActivityId())
		}
		userActivity, err := opuser.FindUserPhotoActivity(ctx, suid, req.GetPhotoActivityId())
		if err != nil {
			return nil, photoOwnershipError("photo activity", req.GetPhotoActivityId(), err)
		}
		if !userActivity.GetIsUnlocked() {
			return nil, viber.FailedPreconditionf("photo activity %q is locked", req.GetPhotoActivityId())
		}
	case penum.PhotoShootingActionType_PhotoShootingActionType_Quest:
		if req.GetPhotoMusicId() == "" || req.GetPhotoStageId() == "" {
			return nil, viber.InvalidArgumentf("photoMusicId and photoStageId are required")
		}
		if _, err := opmaster.FindPhotoQuestMusic(ctx, req.GetPhotoMusicId()); err != nil {
			return nil, photoMasterLookupError("music", req.GetPhotoMusicId(), err)
		}
		if _, err := opmaster.FindPhotoQuestStage(ctx, req.GetPhotoStageId()); err != nil {
			return nil, photoMasterLookupError("stage", req.GetPhotoStageId(), err)
		}
		music, err := opuser.FindUserPhotoMusic(ctx, suid, req.GetPhotoMusicId())
		if err != nil {
			return nil, photoOwnershipError("photo music", req.GetPhotoMusicId(), err)
		}
		if !music.GetIsUnlocked() {
			return nil, viber.FailedPreconditionf("photo music %q is locked", req.GetPhotoMusicId())
		}
		stages, err := opuser.FindAllUserPhotoStages(ctx, suid)
		if err != nil {
			return nil, err
		}
		if !unlockedPhotoStage(stages, req.GetPhotoStageId()) {
			return nil, viber.FailedPreconditionf("photo stage %q is not owned or is locked", req.GetPhotoStageId())
		}
	default:
		return nil, viber.InvalidArgumentf("unsupported photo shooting action type")
	}

	if err := validateOwnedPhotoStyles(ctx, suid, req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds()); err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.PhotoCheckShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}

func (s *PhotoServer) ListShooting(ctx context.Context, _ *papi.Empty) (*papi.PhotoListShootingResponse, error) {
	suid := viber.GetSuid(ctx).String()
	activities, err := opuser.FindAllUserPhotoActivities(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	musics, err := opuser.FindAllUserPhotoMusics(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	stages, err := opuser.FindAllUserPhotoStages(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	expressions, err := opuser.FindAllUserPhotoExpressions(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}

	masterActivities, err := opmaster.FindAllPhotoActivities(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	masterMusics, err := opmaster.FindAllPhotoQuestMusics(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	masterStages, err := opmaster.FindAllPhotoQuestStages(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	masterExpressions, err := opmaster.FindAllPhotoExpressions(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}

	activities = viber.FilterUserPhotoActivities(activities, masterActivities)
	musics = viber.FilterUserPhotoMusics(musics, masterMusics)
	stages = viber.FilterUserPhotoStages(stages, masterStages)
	expressions = viber.FilterUserPhotoExpressions(expressions, masterExpressions)
	sort.Slice(activities, func(i, j int) bool { return activities[i].GetPhotoActivityId() < activities[j].GetPhotoActivityId() })
	sort.Slice(musics, func(i, j int) bool { return musics[i].GetMusicId() < musics[j].GetMusicId() })
	sort.Slice(stages, func(i, j int) bool { return stages[i].GetStageId() < stages[j].GetStageId() })
	sort.Slice(expressions, func(i, j int) bool {
		return expressions[i].GetPhotoExpressionId() < expressions[j].GetPhotoExpressionId()
	})

	return &papi.PhotoListShootingResponse{
		Activities:     activities,
		Musics:         musics,
		Stages:         stages,
		Expressions:    expressions,
		DokanInfo:      &papi.PhotoDokanInfo{Expressions: []*papi.DokanInfo{}},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *PhotoServer) GetDreamBasePhotoEnhanceLevelInfos(
	ctx context.Context,
	req *papi.GetDreamBasePhotoEnhanceLevelInfosRequest,
) (*papi.GetDreamBasePhotoEnhanceLevelInfosResponse, error) {
	if req.GetBasePhotoId() == "" {
		return nil, viber.InvalidArgumentf("basePhotoId is required")
	}
	suid := viber.GetSuid(ctx).String()
	photo, err := opuser.FindUserPhoto(ctx, suid, req.GetBasePhotoId())
	if err != nil {
		return nil, photoOwnershipError("photo", req.GetBasePhotoId(), err)
	}
	if !photo.GetIsDreamBase() || len(photo.GetAbilities()) == 0 {
		return nil, viber.FailedPreconditionf("photo %q is not an enhanced dream base photo", req.GetBasePhotoId())
	}
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	managerLevel, err := opmaster.FindManagerLevel(ctx, user.GetManagerLevel())
	if err != nil {
		return nil, viber.GameplayError(err)
	}

	abilityIDs := make([]string, 0, len(photo.GetAbilities()))
	for _, ability := range photo.GetAbilities() {
		abilityIDs = append(abilityIDs, ability.GetPhotoAbilityId())
	}
	masterAbilities, err := opmaster.FindPhotoAbilities(ctx, abilityIDs)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	masterByID := make(map[string]*pmaster.PhotoAbility, len(masterAbilities))
	for _, ability := range masterAbilities {
		masterByID[ability.GetId()] = ability
	}

	levels := []int32{photo.GetLevel()}
	if photo.GetLevel() <= managerLevel.GetMaxPhotoEnhanceLevel()-5 {
		levels = append(levels, photo.GetLevel()+5)
	}
	levelInfos := make([]*papi.DreamBasePhotoEnhanceLevelInfo, 0, len(levels))
	for _, level := range levels {
		effects, ok := dreamPhotoEffectsAtLevel(photo.GetAbilities(), masterByID, level)
		if !ok {
			if level == photo.GetLevel() {
				return nil, viber.FailedPreconditionf("photo %q has abilities without level data", req.GetBasePhotoId())
			}
			break
		}
		levelInfos = append(levelInfos, &papi.DreamBasePhotoEnhanceLevelInfo{
			Level:            level,
			EffectValueInfos: effects,
		})
	}
	return &papi.GetDreamBasePhotoEnhanceLevelInfosResponse{
		LevelInfos:     levelInfos,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *PhotoServer) CheckStoryEpisodeShooting(
	ctx context.Context,
	req *papi.PhotoCheckStoryEpisodeShootingRequest,
) (*papi.PhotoCheckStoryEpisodeShootingResponse, error) {
	storyID, err := resolvePhotoStoryEpisode(ctx, req.GetStoryPartId(), req.GetChapter(), req.GetRoute(), req.GetEpisode())
	if err != nil {
		return nil, err
	}
	story, err := opuser.FindUserStory(ctx, viber.GetSuid(ctx).String(), storyID)
	if err != nil {
		return nil, photoOwnershipError("story", storyID, err)
	}
	if story.GetIsInvalid() ||
		story.GetStatusType() != penum.StoryStatusType_StoryStatusType_Read &&
			!story.GetIsUseActivatePhotoShootingStoryTicket() {
		return nil, viber.FailedPreconditionf("story %q is not available for shooting", storyID)
	}
	return &papi.PhotoCheckStoryEpisodeShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}

func (s *PhotoServer) CheckCardStoryShooting(
	ctx context.Context,
	req *papi.PhotoCheckCardStoryShootingRequest,
) (*papi.PhotoCheckCardStoryShootingResponse, error) {
	if req.GetCardId() == "" || req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("cardId and positive number are required")
	}
	card, err := opmaster.FindCard(ctx, req.GetCardId())
	if err != nil {
		return nil, photoMasterLookupError("card", req.GetCardId(), err)
	}
	if int(req.GetNumber()) > len(card.GetStories()) {
		return nil, viber.InvalidArgumentf("card story number is out of range")
	}
	suid := viber.GetSuid(ctx).String()
	if _, err := opuser.FindUserCard(ctx, suid, req.GetCardId()); err != nil {
		return nil, photoOwnershipError("card", req.GetCardId(), err)
	}
	storyID := card.GetStories()[req.GetNumber()-1].GetStoryId()
	story, err := opuser.FindUserStory(ctx, suid, storyID)
	if err != nil {
		return nil, photoOwnershipError("card story", storyID, err)
	}
	if story.GetIsInvalid() || story.GetStatusType() == penum.StoryStatusType_StoryStatusType_Unknown {
		return nil, viber.FailedPreconditionf("card story %q is unavailable", storyID)
	}
	return &papi.PhotoCheckCardStoryShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}

func (s *PhotoServer) CheckExpressionShooting(
	ctx context.Context,
	req *papi.PhotoCheckExpressionShootingRequest,
) (*papi.PhotoCheckExpressionShootingResponse, error) {
	if req.GetPhotoExpressionId() == "" {
		return nil, viber.InvalidArgumentf("photoExpressionId is required")
	}
	if _, err := opmaster.FindPhotoExpression(ctx, req.GetPhotoExpressionId()); err != nil {
		return nil, photoMasterLookupError("expression", req.GetPhotoExpressionId(), err)
	}
	suid := viber.GetSuid(ctx).String()
	expressions, err := opuser.FindAllUserPhotoExpressions(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if !unlockedPhotoExpression(expressions, req.GetPhotoExpressionId()) {
		return nil, viber.FailedPreconditionf("photo expression %q is not owned or is locked", req.GetPhotoExpressionId())
	}

	singularSet := req.GetCharacterId() != "" || req.GetCostumeId() != "" || req.GetHairId() != ""
	if singularSet && (req.GetCharacterId() == "" || req.GetCostumeId() == "" || req.GetHairId() == "") {
		return nil, viber.InvalidArgumentf("characterId, costumeId, and hairId must be provided together")
	}
	if err := validateOptionalPhotoStyleLists(req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds()); err != nil {
		return nil, err
	}
	if !singularSet && len(req.GetCharacterIds()) == 0 {
		return nil, viber.InvalidArgumentf("at least one character is required")
	}
	if singularSet {
		if err := validateOwnedPhotoStyles(ctx, suid, []string{req.GetCharacterId()}, []string{req.GetCostumeId()}, []string{req.GetHairId()}); err != nil {
			return nil, viber.GameplayError(err)
		}
	}
	if len(req.GetCharacterIds()) > 0 {
		if err := validateOwnedPhotoStyles(ctx, suid, req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds()); err != nil {
			return nil, viber.GameplayError(err)
		}
	}
	return &papi.PhotoCheckExpressionShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}

func (s *PhotoServer) CheckExcursionShooting(
	ctx context.Context,
	req *papi.PhotoCheckExcursionShootingRequest,
) (*papi.PhotoCheckExcursionShootingResponse, error) {
	if req.GetExcursionPlaceId() == "" || req.GetCharacterId() == "" || req.GetCostumeId() == "" || req.GetHairId() == "" {
		return nil, viber.InvalidArgumentf("excursionPlaceId, characterId, costumeId, and hairId are required")
	}
	if _, err := opmaster.FindExcursionPlace(ctx, req.GetExcursionPlaceId()); err != nil {
		return nil, photoMasterLookupError("excursion place", req.GetExcursionPlaceId(), err)
	}
	suid := viber.GetSuid(ctx).String()
	if err := validateOwnedPhotoStyles(ctx, suid, []string{req.GetCharacterId()}, []string{req.GetCostumeId()}, []string{req.GetHairId()}); err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.PhotoCheckExcursionShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}

func photoFirebaseDownloadURL(name string, metadata *umodel.UserPhotoImageMetadata) string {
	query := url.Values{"alt": []string{"media"}}
	if metadata != nil && metadata.DownloadTokens != "" {
		query.Set("token", metadata.DownloadTokens)
	}
	return "https://firebasestorage.googleapis.com/v0/b/" + photoFirebaseBucket + "/o/" + url.PathEscape(name) + "?" + query.Encode()
}

func isPhotoAssetID(value string) bool {
	if value == "" || value == "." || value == ".." || len(value) > 251 {
		return false
	}
	for index := range len(value) {
		character := value[index]
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			strings.ContainsRune("-_.~", rune(character)) {
			continue
		}
		return false
	}
	return true
}

func validatePhotoStyleLists(characterIDs, costumeIDs, hairIDs []string) error {
	if len(characterIDs) == 0 {
		return viber.InvalidArgumentf("at least one character is required")
	}
	return validateOptionalPhotoStyleLists(characterIDs, costumeIDs, hairIDs)
}

func validateOptionalPhotoStyleLists(characterIDs, costumeIDs, hairIDs []string) error {
	if len(characterIDs) != len(costumeIDs) || len(characterIDs) != len(hairIDs) {
		return viber.InvalidArgumentf("characterIds, costumeIds, and hairIds must have equal lengths")
	}
	return nil
}

func validateOwnedPhotoStyles(ctx context.Context, suid string, characterIDs, costumeIDs, hairIDs []string) error {
	for index, characterID := range characterIDs {
		if characterID == "" || costumeIDs[index] == "" || hairIDs[index] == "" {
			return viber.InvalidArgumentf("photo character, costume, and hair IDs cannot be empty")
		}
		if _, err := opuser.FindUserCharacter(ctx, suid, characterID); err != nil {
			return photoOwnershipError("character", characterID, err)
		}
		if _, err := opuser.FindUserCostume(ctx, suid, costumeIDs[index]); err != nil {
			return photoOwnershipError("costume", costumeIDs[index], err)
		}
		if _, err := opuser.FindUserHair(ctx, suid, hairIDs[index]); err != nil {
			return photoOwnershipError("hair", hairIDs[index], err)
		}
		costume, err := opmaster.FindCostume(ctx, costumeIDs[index])
		if err != nil {
			return photoMasterLookupError("costume", costumeIDs[index], err)
		}
		hair, err := opmaster.FindHair(ctx, hairIDs[index])
		if err != nil {
			return photoMasterLookupError("hair", hairIDs[index], err)
		}
		if costume.GetCharacterId() != characterID || hair.GetCharacterId() != characterID {
			return viber.FailedPreconditionf("costume and hair must belong to character %q", characterID)
		}
	}
	return nil
}

func unlockedPhotoStage(stages []*papi.PhotoStage, stageID string) bool {
	for _, stage := range stages {
		if stage.GetStageId() == stageID {
			return stage.GetIsUnlocked()
		}
	}
	return false
}

func unlockedPhotoExpression(expressions []*papi.PhotoExpression, expressionID string) bool {
	for _, expression := range expressions {
		if expression.GetPhotoExpressionId() == expressionID {
			return expression.GetIsUnlocked()
		}
	}
	return false
}

func resolvePhotoStoryEpisode(ctx context.Context, partID string, chapter, route, episode int32) (string, error) {
	if partID == "" || chapter <= 0 || route <= 0 || episode <= 0 {
		return "", viber.InvalidArgumentf("storyPartId and positive chapter, route, and episode are required")
	}
	part, err := opmaster.FindStoryPart(ctx, partID)
	if err != nil {
		return "", photoMasterLookupError("story part", partID, err)
	}
	for _, candidateChapter := range part.GetChapters() {
		if candidateChapter.GetChapter() != chapter || candidateChapter.GetRoute() != route {
			continue
		}
		for _, candidateEpisode := range candidateChapter.GetEpisodes() {
			if candidateEpisode.GetEpisode() == episode && candidateEpisode.GetStoryId() != "" {
				return candidateEpisode.GetStoryId(), nil
			}
		}
	}
	return "", viber.InvalidArgumentf("unknown story episode")
}

func dreamPhotoEffectsAtLevel(
	abilities []*ptransaction.UserPhotoAbility,
	master map[string]*pmaster.PhotoAbility,
	level int32,
) ([]*papi.DreamBasePhotoEnhanceLevelEffectValueInfo, bool) {
	effects := make([]*papi.DreamBasePhotoEnhanceLevelEffectValueInfo, 0, len(abilities))
	for _, ability := range abilities {
		masterAbility, exists := master[ability.GetPhotoAbilityId()]
		if !exists {
			return nil, false
		}
		var baseValue int32
		levelFound := false
		for _, candidate := range masterAbility.GetPhotoAbilityLevels() {
			if candidate.GetLevel() == level {
				baseValue = candidate.GetValue()
				levelFound = true
				break
			}
		}
		if !levelFound {
			return nil, false
		}
		value := int64(baseValue)
		gradeFound := false
		for _, grade := range masterAbility.GetPhotoAbilityGrades() {
			if grade.GetGrade() == ability.GetGrade() {
				value = value*int64(grade.GetBonusPermil())/1000 + int64(grade.GetBonusValue())
				gradeFound = true
				break
			}
		}
		if !gradeFound {
			return nil, false
		}
		effects = append(effects, &papi.DreamBasePhotoEnhanceLevelEffectValueInfo{
			PhotoAbilityId: ability.GetPhotoAbilityId(),
			EffectValue:    value,
		})
	}
	return effects, true
}

func photoMasterLookupError(kind, id string, err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return viber.InvalidArgumentf("unknown %s %q", kind, id)
	}
	return err
}

func photoOwnershipError(kind, id string, err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return viber.FailedPreconditionf("%s %q is not owned or unlocked", kind, id)
	}
	return err
}
