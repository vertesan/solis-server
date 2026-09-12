package user

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (s *UserServer) Get(ctx context.Context, _ *papi.Empty) (*papi.UserGetResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.UserGetResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		if _, err := viber.ReconcileCardSupportLevel(ctx, suid, user, &papi.UpdatedData{}); err != nil {
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
		now := time.Now().UnixMilli()
		for _, character := range characters {
			if _, err := viber.ReconcileActivityCharacterStamina(ctx, suid, character, now); err != nil {
				return nil, err
			}
		}
		costumes, err := opuser.FindAllUserCostumes(ctx, suid)
		if err != nil {
			return nil, err
		}
		costumeIDs := make([]string, 0, len(costumes))
		for _, costume := range costumes {
			costumeIDs = append(costumeIDs, costume.GetCostumeId())
		}
		masterCostumes, err := opmaster.FindCostumes(ctx, costumeIDs)
		if err != nil {
			return nil, err
		}
		sdCostumeIDs := make(map[string]struct{})
		for _, costume := range masterCostumes {
			if strings.HasPrefix(costume.GetCostumeTypeId(), "sd-") {
				sdCostumeIDs[costume.GetId()] = struct{}{}
			}
		}
		visibleCostumes := make([]*ptransaction.UserCostume, 0, len(costumes))
		for _, costume := range costumes {
			if _, sdOnly := sdCostumeIDs[costume.GetCostumeId()]; !sdOnly {
				visibleCostumes = append(visibleCostumes, costume)
			}
		}
		costumes = visibleCostumes
		accessories, err := opuser.FindAllUserAccessories(ctx, suid)
		if err != nil {
			return nil, err
		}
		if _, err := viber.UnlockManagerLevelPhotoAbilities(ctx, suid, user.GetManagerLevel()); err != nil {
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
		gachaInfos, err := viber.ReconcileCurrentGachaInfos(ctx, suid, now)
		if err != nil {
			return nil, err
		}
		gachaButtons := flattenGachaButtons(gachaInfos)
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
		for i, love := range loves {
			if slices.Contains(moshikoi.SupportedLoveIDs(), love.GetLoveId()) {
				loves[i] = moshikoi.VisibleLove(love)
			}
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
		excursionGazeReactions, err := opuser.FindAllUserExcursionGazeReactions(ctx, suid)
		if err != nil {
			return nil, err
		}
		var staffTrainingReset *ptransaction.UserStaffTrainingReset
		staffTrainingReset, err = opuser.FindUserStaffTrainingReset(ctx, suid)
		if err != nil {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return nil, err
			}
		} else if staffTrainingReset.GetStaffTrainingResetCount() != 0 {
			// Staff resets are free and unlimited on the private server. Normalize
			// imported official state before the client can hide a third reset.
			staffTrainingReset.StaffTrainingResetCount = 0
			if err := opuser.UpsertUserStaffTrainingReset(ctx, suid, staffTrainingReset); err != nil {
				return nil, err
			}
		}
		userPublic, err := opuser.FindUserPublic(ctx, suid)
		if err != nil {
			return nil, err
		}

		response = &papi.UserGetResponse{
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
			ExcursionGazeReactions:   excursionGazeReactions,
			StaffTrainingReset:       staffTrainingReset,
			UserPublic:               userPublic,
		}
		return response, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func flattenGachaButtons(infos []*papi.GachaInfo) []*ptransaction.UserGachaButton {
	buttons := make([]*ptransaction.UserGachaButton, 0)
	for _, info := range infos {
		for _, button := range info.GetButtons() {
			buttons = append(buttons, &ptransaction.UserGachaButton{
				GachaButtonId: button.GetId(),
				DrawTime:      button.GetDrawTime(),
				TodayCount:    button.GetTodayCount(),
				TotalCount:    button.GetTotalCount(),
			})
		}
	}
	return buttons
}
