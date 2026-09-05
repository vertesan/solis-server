package servers

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

type AccessoryServer struct {
	papi.UnimplementedAccessoryServer
}

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

func (s *AccessoryServer) LimitBreak(ctx context.Context, req *papi.AccessoryLimitBreakRequest) (*papi.AccessoryLimitBreakResponse, error) {
	if req.GetAccessoryId() == "" {
		return nil, viber.InvalidArgumentf("accessoryId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.AccessoryLimitBreakResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		masterAccessory, err := opmaster.FindAccessory(ctx, req.AccessoryId)
		if err != nil {
			return nil, err
		}
		if masterAccessory.LimitBreakPhase <= 0 || masterAccessory.LimitBreakConsumptionId == "" {
			return nil, viber.FailedPreconditionf("accessory is not a limit-break target")
		}
		target, err := opuser.FindUserAccessory(ctx, suid, req.AccessoryId)
		if err != nil {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return nil, err
			}
			target = &ptransaction.UserAccessory{AccessoryId: req.AccessoryId}
		}
		materialMaster, err := opmaster.FindAccessoryByClassificationAndPhase(ctx, masterAccessory, masterAccessory.LimitBreakPhase-1)
		if err != nil {
			return nil, err
		}
		material, err := opuser.FindUserAccessory(ctx, suid, materialMaster.Id)
		if err != nil {
			return nil, err
		}
		if material.Amount <= 0 {
			return nil, viber.FailedPreconditionf("previous limit-break phase accessory is not owned")
		}
		consumption, err := opmaster.FindConsumption(ctx, masterAccessory.LimitBreakConsumptionId)
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		for _, resource := range consumption.Resources {
			if resource.Amount <= 0 {
				return nil, fmt.Errorf("limit-break master consumption must be positive")
			}
			switch resource.ResourceType {
			case penum.ResourceType_ResourceType_Item:
				if len(resource.ResourceIds) != 1 {
					return nil, fmt.Errorf("limit-break item consumption must name exactly one item")
				}
				item, err := viber.ConsumePermanentItem(ctx, suid, resource.ResourceIds[0], int64(resource.Amount))
				if err != nil {
					return nil, err
				}
				updated.Items = append(updated.Items, item)
			case penum.ResourceType_ResourceType_Gold:
				point, err := viber.ConsumeGold(ctx, suid, int64(resource.Amount))
				if err != nil {
					return nil, err
				}
				updated.Points = append(updated.Points, point)
			case penum.ResourceType_ResourceType_Accessory:
				if len(resource.ResourceIds) != 1 {
					return nil, fmt.Errorf("limit-break accessory consumption must name exactly one accessory")
				}
				accessory, err := opuser.FindUserAccessory(ctx, suid, resource.ResourceIds[0])
				if err != nil {
					return nil, err
				}
				if accessory.Amount < int64(resource.Amount) {
					return nil, viber.FailedPreconditionf("insufficient accessory %s", resource.ResourceIds[0])
				}
				accessory.Amount -= int64(resource.Amount)
				if err := opuser.UpsertUserAccessory(ctx, suid, accessory); err != nil {
					return nil, err
				}
				if accessory.AccessoryId == material.AccessoryId {
					material = accessory
				}
				if accessory.AccessoryId == target.AccessoryId {
					target = accessory
				}
				updated.Accessories = append(updated.Accessories, accessory)
			default:
				return nil, fmt.Errorf("unsupported limit-break resource type %s", resource.ResourceType)
			}
		}
		if material.Amount <= 0 {
			return nil, viber.FailedPreconditionf("insufficient previous-phase accessory")
		}
		material.Amount--
		target.Amount++
		if err := opuser.UpsertUserAccessory(ctx, suid, material); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserAccessory(ctx, suid, target); err != nil {
			return nil, err
		}
		if !containsAccessory(updated.Accessories, target.AccessoryId) {
			updated.Accessories = append(updated.Accessories, target)
		}
		if !containsAccessory(updated.Accessories, material.AccessoryId) {
			updated.Accessories = append(updated.Accessories, material)
		}
		deckPositions, leaguePositions, err := replaceAccessoryInDecks(ctx, suid, material.AccessoryId, target.AccessoryId)
		if err != nil {
			return nil, err
		}
		updated.DeckPositions = deckPositions
		updated.LeagueDeckPositions = leaguePositions
		response = &papi.AccessoryLimitBreakResponse{CommonResponse: viber.NewCommonResponse(updated)}
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
	reserved := viber.ReservedAccessoryAmount(materialMaster.Id, deckPositions, leaguePositions)
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

func containsAccessory(accessories []*ptransaction.UserAccessory, id string) bool {
	for _, accessory := range accessories {
		if accessory.AccessoryId == id {
			return true
		}
	}
	return false
}

func replaceAccessoryInDecks(ctx context.Context, suid, oldID, newID string) ([]*ptransaction.UserDeckPosition, []*ptransaction.UserLeagueDeckPosition, error) {
	var updatedNormal []*ptransaction.UserDeckPosition
	positions, err := opuser.FindAllUserDeckPositions(ctx, suid)
	if err != nil {
		return nil, nil, err
	}
	for _, position := range positions {
		changed := false
		if position.Part1AccessoryId == oldID {
			position.Part1AccessoryId, changed = newID, true
		}
		if position.Part2AccessoryId == oldID {
			position.Part2AccessoryId, changed = newID, true
		}
		if changed {
			updatedNormal = append(updatedNormal, position)
		}
	}
	if err := opuser.UpsertManyUserDeckPositions(ctx, suid, updatedNormal); err != nil {
		return nil, nil, err
	}
	var updatedLeague []*ptransaction.UserLeagueDeckPosition
	leaguePositions, err := opuser.FindAllUserLeagueDeckPositions(ctx, suid)
	if err != nil {
		return nil, nil, err
	}
	for _, position := range leaguePositions {
		changed := false
		if position.Part1AccessoryId == oldID {
			position.Part1AccessoryId, changed = newID, true
		}
		if position.Part2AccessoryId == oldID {
			position.Part2AccessoryId, changed = newID, true
		}
		if changed {
			updatedLeague = append(updatedLeague, position)
		}
	}
	if err := opuser.UpsertManyUserLeagueDeckPositions(ctx, suid, updatedLeague); err != nil {
		return nil, nil, err
	}
	return updatedNormal, updatedLeague, nil
}
