package accessory

import (
	"context"
	"errors"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (s *AccessoryServer) Enhance(ctx context.Context, req *papi.AccessoryEnhanceRequest) (*papi.AccessoryEnhanceResponse, error) {
	if req.GetAccessoryId() == "" || req.GetCount() <= 0 {
		return nil, viber.InvalidArgumentf("accessoryId and a positive count are required")
	}
	if req.GetMaterialType() < penum.AccessoryEnhanceMaterialType_AccessoryEnhanceMaterialType_ExcludeAllDeck ||
		req.GetMaterialType() > penum.AccessoryEnhanceMaterialType_AccessoryEnhanceMaterialType_AllUserAccessory {
		return nil, viber.InvalidArgumentf("unknown accessory enhancement material type")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.AccessoryEnhanceResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		target, err := opuser.FindUserAccessory(ctx, suid, req.AccessoryId)
		if err != nil {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return nil, err
			}
			target = &ptransaction.UserAccessory{AccessoryId: req.AccessoryId}
		}
		masterAccessory, err := opmaster.FindAccessory(ctx, req.AccessoryId)
		if err != nil {
			return nil, err
		}
		if masterAccessory.LimitBreakPhase != 0 {
			return nil, viber.FailedPreconditionf("limit-broken accessories cannot be produced by enhancement")
		}
		materialMaster, err := opmaster.FindAccessoryEnhancementMaterial(ctx, masterAccessory)
		if err != nil {
			return nil, err
		}
		deckPositions, err := opuser.FindAllUserDeckPositions(ctx, suid)
		if err != nil {
			return nil, err
		}
		leaguePositions, err := opuser.FindAllUserLeagueDeckPositions(ctx, suid)
		if err != nil {
			return nil, err
		}
		required := int64(req.Count)
		settingID, err := config.GetGameSettingId()
		if err != nil {
			return nil, err
		}
		setting, err := opmaster.FindSetting(ctx, settingID)
		if err != nil {
			return nil, err
		}
		required *= int64(setting.AccessoryEnhancesRequiredMaterialAmount)
		if required <= 0 {
			return nil, fmt.Errorf("master enhancement material requirement is invalid")
		}
		material, err := selectAccessoryMaterial(ctx, suid, materialMaster, req.MaterialType, required, deckPositions, leaguePositions)
		if err != nil {
			return nil, err
		}
		enhancement, err := opmaster.FindAccessoryEnhancement(ctx, masterAccessory.Rarity)
		if err != nil {
			return nil, err
		}
		goldCost := enhancement.RequiredGold * int64(req.Count)
		if goldCost <= 0 {
			return nil, fmt.Errorf("master enhancement gold requirement is invalid")
		}
		gold, err := viber.ConsumeGold(ctx, suid, goldCost)
		if err != nil {
			return nil, err
		}
		material.Amount -= required
		if err := opuser.UpsertUserAccessory(ctx, suid, material); err != nil {
			return nil, err
		}
		updatedDecks, updatedLeagueDecks, err := clearConsumedAccessoryFromDecks(
			ctx, suid, material.AccessoryId, material.Amount, req.MaterialType, deckPositions, leaguePositions,
		)
		if err != nil {
			return nil, err
		}
		target.Amount += int64(req.Count)
		if err := opuser.UpsertUserAccessory(ctx, suid, target); err != nil {
			return nil, err
		}
		missions, err := viber.AdvanceUserMissionsWithAmount(
			ctx,
			suid,
			penum.MissionType_MissionType_AccessoryEnhanceCount,
			func(mission *pmaster.Mission) int64 {
				if mission.CategoryType == penum.MissionCategoryType_MissionCategoryType_Daily {
					return int64(req.Count)
				}
				return required
			},
			target.AccessoryId,
			masterAccessory.CharacterId,
			masterAccessory.CharacterGroupId,
		)
		if err != nil {
			return nil, err
		}
		response = &papi.AccessoryEnhanceResponse{
			IsUnsetLeague: len(updatedLeagueDecks) > 0,
			CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
				Accessories:         []*ptransaction.UserAccessory{material, target},
				Points:              []*ptransaction.UserPoint{gold},
				DeckPositions:       updatedDecks,
				LeagueDeckPositions: updatedLeagueDecks,
				Missions:            missions,
			}),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func selectAccessoryMaterial(
	ctx context.Context,
	suid string,
	materialMaster *pmaster.Accessory,
	materialType penum.AccessoryEnhanceMaterialType,
	required int64,
	deckPositions []*ptransaction.UserDeckPosition,
	leaguePositions []*ptransaction.UserLeagueDeckPosition,
) (*ptransaction.UserAccessory, error) {
	accessory, err := opuser.FindUserAccessory(ctx, suid, materialMaster.Id)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, viber.FailedPreconditionf("enhancement material %s is not owned", materialMaster.Id)
		}
		return nil, err
	}
	if materialType != penum.AccessoryEnhanceMaterialType_AccessoryEnhanceMaterialType_ExcludeAllDeck {
		deckPositions = nil
	}
	if materialType == penum.AccessoryEnhanceMaterialType_AccessoryEnhanceMaterialType_AllUserAccessory {
		leaguePositions = nil
	}
	reserved := reservedAccessoryAmount(materialMaster.Id, deckPositions, leaguePositions)
	if accessory.Amount-reserved >= required {
		return accessory, nil
	}
	return nil, viber.FailedPreconditionf("insufficient eligible previous-rarity accessories")
}

func clearConsumedAccessoryFromDecks(
	ctx context.Context,
	suid, accessoryID string,
	remaining int64,
	materialType penum.AccessoryEnhanceMaterialType,
	deckPositions []*ptransaction.UserDeckPosition,
	leaguePositions []*ptransaction.UserLeagueDeckPosition,
) ([]*ptransaction.UserDeckPosition, []*ptransaction.UserLeagueDeckPosition, error) {
	if materialType == penum.AccessoryEnhanceMaterialType_AccessoryEnhanceMaterialType_ExcludeAllDeck {
		return nil, nil, nil
	}
	var updatedDecks []*ptransaction.UserDeckPosition
	usedByDeck := make(map[int32]int64)
	for _, position := range deckPositions {
		changed := false
		if position.Part1AccessoryId == accessoryID {
			usedByDeck[position.Number]++
			if usedByDeck[position.Number] > remaining {
				position.Part1AccessoryId, changed = "", true
			}
		}
		if position.Part2AccessoryId == accessoryID {
			usedByDeck[position.Number]++
			if usedByDeck[position.Number] > remaining {
				position.Part2AccessoryId, changed = "", true
			}
		}
		if changed {
			updatedDecks = append(updatedDecks, position)
		}
	}
	if err := opuser.UpsertManyUserDeckPositions(ctx, suid, updatedDecks); err != nil {
		return nil, nil, err
	}
	if materialType != penum.AccessoryEnhanceMaterialType_AccessoryEnhanceMaterialType_AllUserAccessory {
		return updatedDecks, nil, nil
	}
	var updatedLeagueDecks []*ptransaction.UserLeagueDeckPosition
	type leagueDeckKey struct {
		season string
		typeID penum.LeagueDeckType
	}
	usedByLeagueDeck := make(map[leagueDeckKey]int64)
	for _, position := range leaguePositions {
		changed := false
		key := leagueDeckKey{position.LeagueSeasonId, position.DeckType}
		if position.Part1AccessoryId == accessoryID {
			usedByLeagueDeck[key]++
			if usedByLeagueDeck[key] > remaining {
				position.Part1AccessoryId, changed = "", true
			}
		}
		if position.Part2AccessoryId == accessoryID {
			usedByLeagueDeck[key]++
			if usedByLeagueDeck[key] > remaining {
				position.Part2AccessoryId, changed = "", true
			}
		}
		if changed {
			updatedLeagueDecks = append(updatedLeagueDecks, position)
		}
	}
	if err := opuser.UpsertManyUserLeagueDeckPositions(ctx, suid, updatedLeagueDecks); err != nil {
		return nil, nil, err
	}
	return updatedDecks, updatedLeagueDecks, nil
}
