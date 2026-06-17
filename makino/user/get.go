package user

import (
	"context"
	"errors"
	"vertesan/solis-server/db/store/opuser"
	"vertesan/solis-server/proto/papi"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

func GetUser(ctx context.Context, suid string) (*papi.UserGetResponse, error) {
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, err
	}
	items, err := opuser.FindAllUserItems(ctx, suid)
	if err != nil {
		return nil, err
	}
	cards, err := opuser.FindAllUserCards(ctx, suid)
	if err != nil {
		return nil, err
	}
	characters, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		return nil, err
	}
	costumes, err := opuser.FindAllUserCostumes(ctx, suid)
	if err != nil {
		return nil, err
	}
	accessories, err := opuser.FindAllUserAccessories(ctx, suid)
	if err != nil {
		return nil, err
	}
	photos, err := opuser.FindAllUserPhotos(ctx, suid)
	if err != nil {
		return nil, err
	}
	decks, err := opuser.FindAllUserDecks(ctx, suid)
	if err != nil {
		return nil, err
	}
	deckPositions, err := opuser.FindAllUserDeckPositions(ctx, suid)
	if err != nil {
		return nil, err
	}
	stories, err := opuser.FindAllUserStories(ctx, suid)
	if err != nil {
		return nil, err
	}
	points, err := opuser.FindAllUserPoints(ctx, suid)
	if err != nil {
		return nil, err
	}
	staffs, err := opuser.FindAllUserStaffs(ctx, suid)
	if err != nil {
		return nil, err
	}
	messages, err := opuser.FindAllUserMessages(ctx, suid)
	if err != nil {
		return nil, err
	}
	telephones, err := opuser.FindAllUserTelephones(ctx, suid)
	if err != nil {
		return nil, err
	}
	missions, err := opuser.FindAllUserMissions(ctx, suid)
	if err != nil {
		return nil, err
	}
	profile, err := opuser.FindUserProfile(ctx, suid)
	if err != nil {
		return nil, err
	}
	emblems, err := opuser.FindAllUserEmblems(ctx, suid)
	if err != nil {
		return nil, err
	}
	characterMusics, err := opuser.FindAllUserCharacterMusics(ctx, suid)
	if err != nil {
		return nil, err
	}
	balance, err := opuser.FindUserBalance(ctx, suid)
	if err != nil {
		return nil, err
	}
	totalCount, err := opuser.FindUserTotalCount(ctx, suid)
	if err != nil {
		return nil, err
	}
	areas, err := opuser.FindAllUserAreas(ctx, suid)
	if err != nil {
		return nil, err
	}
	quests, err := opuser.FindAllUserQuests(ctx, suid)
	if err != nil {
		return nil, err
	}
	gachaButtons, err := opuser.FindAllUserGachaButtons(ctx, suid)
	if err != nil {
		return nil, err
	}
	homeTalks, err := opuser.FindAllUserHomeTalks(ctx, suid)
	if err != nil {
		return nil, err
	}
	photoReport, err := opuser.FindUserPhotoReport(ctx, suid)
	if err != nil {
		return nil, err
	}
	cardSupports, err := opuser.FindAllUserCardSupports(ctx, suid)
	if err != nil {
		return nil, err
	}
	messageSchedules, err := opuser.FindAllUserMessageSchedules(ctx, suid)
	if err != nil {
		return nil, err
	}
	notifications, err := opuser.FindAllUserNotifications(ctx, suid)
	if err != nil {
		return nil, err
	}
	hierarchy, err := opuser.FindUserHierarchy(ctx, suid)
	if err != nil {
		return nil, err
	}
	tutorials, err := opuser.FindAllUserTutorials(ctx, suid)
	if err != nil {
		return nil, err
	}
	homePositions, err := opuser.FindAllUserHomePositions(ctx, suid)
	if err != nil {
		return nil, err
	}
	musics, err := opuser.FindAllUserMusics(ctx, suid)
	if err != nil {
		return nil, err
	}
	decorations, err := opuser.FindAllUserDecorations(ctx, suid)
	if err != nil {
		return nil, err
	}
	photoRecipes, err := opuser.FindAllUserPhotoRecipes(ctx, suid)
	if err != nil {
		return nil, err
	}
	buddy, err := opuser.FindUserBuddy(ctx, suid)
	if err != nil {
		return nil, err
	}
	var invite *ptransaction.UserInvite
	invite, err = opuser.FindUserInvite(ctx, suid)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
	}
	hairs, err := opuser.FindAllUserHairs(ctx, suid)
	if err != nil {
		return nil, err
	}
	var activityFanEventProgress *ptransaction.UserActivityFanEventProgress
	activityFanEventProgress, err = opuser.FindUserActivityFanEventProgress(ctx, suid)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
	}
	loves, err := opuser.FindAllUserLoves(ctx, suid)
	if err != nil {
		return nil, err
	}
	leagueDeckPositions, err := opuser.FindAllUserLeagueDeckPositions(ctx, suid)
	if err != nil {
		return nil, err
	}
	duties, err := opuser.FindAllUserDuties(ctx, suid)
	if err != nil {
		return nil, err
	}
	dutyPoints, err := opuser.FindAllUserDutyPoints(ctx, suid)
	if err != nil {
		return nil, err
	}
	company, err := opuser.FindUserCompany(ctx, suid)
	if err != nil {
		return nil, err
	}
	showcases, err := opuser.FindAllUserShowcases(ctx, suid)
	if err != nil {
		return nil, err
	}
	showcaseLike, err := opuser.FindUserShowcaseLike(ctx, suid)
	if err != nil {
		return nil, err
	}
	showcaseMusics, err := opuser.FindAllUserShowcaseMusics(ctx, suid)
	if err != nil {
		return nil, err
	}
	showcaseToys, err := opuser.FindAllUserShowcaseToys(ctx, suid)
	if err != nil {
		return nil, err
	}
	showcaseMysets, err := opuser.FindAllUserShowcaseMysets(ctx, suid)
	if err != nil {
		return nil, err
	}
	tourAreas, err := opuser.FindAllUserTourAreas(ctx, suid)
	if err != nil {
		return nil, err
	}
	photoPoses, err := opuser.FindAllUserPhotoPoses(ctx, suid)
	if err != nil {
		return nil, err
	}
	var staffTrainingReset *ptransaction.UserStaffTrainingReset
	staffTrainingReset, err = opuser.FindUserStaffTrainingReset(ctx, suid)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
	}
	userPublic, err := opuser.FindUserPublic(ctx, suid)
	if err != nil {
		return nil, err
	}

	return &papi.UserGetResponse{
		User:                     user,
		Items:                    items,
		Cards:                    cards,
		Characters:               characters,
		Costumes:                 costumes,
		Accessories:              accessories,
		Photos:                   photos,
		Decks:                    decks,
		DeckPositions:            deckPositions,
		Stories:                  stories,
		Points:                   points,
		Staffs:                   staffs,
		Messages:                 messages,
		Telephones:               telephones,
		Missions:                 missions,
		Profile:                  profile,
		Emblems:                  emblems,
		CharacterMusics:          characterMusics,
		Balance:                  balance,
		TotalCount:               totalCount,
		Areas:                    areas,
		Quests:                   quests,
		GachaButtons:             gachaButtons,
		HomeTalks:                homeTalks,
		PhotoReport:              photoReport,
		CardSupports:             cardSupports,
		MessageSchedules:         messageSchedules,
		Notifications:            notifications,
		Hierarchy:                hierarchy,
		Tutorials:                tutorials,
		HomePositions:            homePositions,
		Musics:                   musics,
		Decorations:              decorations,
		PhotoRecipes:             photoRecipes,
		Buddy:                    buddy,
		Invite:                   invite,
		Hairs:                    hairs,
		ActivityFanEventProgress: activityFanEventProgress,
		Loves:                    loves,
		LeagueDeckPositions:      leagueDeckPositions,
		Duties:                   duties,
		DutyPoints:               dutyPoints,
		Company:                  company,
		Showcases:                showcases,
		ShowcaseLike:             showcaseLike,
		ShowcaseMusics:           showcaseMusics,
		ShowcaseToys:             showcaseToys,
		ShowcaseMysets:           showcaseMysets,
		TourAreas:                tourAreas,
		PhotoPoses:               photoPoses,
		StaffTrainingReset:       staffTrainingReset,
		UserPublic:               userPublic,
	}, nil
}
