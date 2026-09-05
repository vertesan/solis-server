package card

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *CardServer) SkillEnhance(ctx context.Context, req *papi.CardSkillEnhanceRequest) (*papi.CardSkillEnhanceResponse, error) {
	if req.GetCardId() == "" || req.GetSkillId() == "" {
		return nil, viber.InvalidArgumentf("cardId and skillId are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CardSkillEnhanceResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		card, err := opuser.FindUserCard(ctx, suid, req.CardId)
		if err != nil {
			return nil, err
		}
		level, setLevel, ok := cardSkillLevel(card, req.SkillId)
		if !ok {
			return nil, viber.FailedPreconditionf("skill does not belong to the card or is not unlocked")
		}
		skill, err := opmaster.FindSkill(ctx, req.SkillId)
		if err != nil {
			return nil, err
		}
		next, err := nextSkillLevel(skill, level, card.Level)
		if err != nil {
			return nil, err
		}
		item, err := viber.ConsumePermanentItem(ctx, suid, cardSkillItemID, int64(next.RequiredItemAmount))
		if err != nil {
			return nil, err
		}
		setLevel(next.Level)
		if !card.Supported {
			setBaseSkillLevel(card, req.SkillId, next.Level)
		}
		if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
			return nil, err
		}
		missions, err := viber.AdvanceUserMissions(
			ctx,
			suid,
			penum.MissionType_MissionType_CardSkillLevelUpCount,
			1,
			card.CardId,
			req.SkillId,
		)
		if err != nil {
			return nil, err
		}
		masterCard, err := opmaster.FindCard(ctx, card.CardId)
		if err != nil {
			return nil, err
		}
		characterMissions, err := reachCharacterCardTrainingMissions(ctx, suid, masterCard.CharacterId)
		if err != nil {
			return nil, err
		}
		missions = append(missions, characterMissions...)
		response = &papi.CardSkillEnhanceResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			Items: []*ptransaction.UserItem{item}, Cards: []*ptransaction.UserCard{card}, Missions: missions,
		})}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func cardSkillLevel(card *ptransaction.UserCard, id string) (int32, func(int32), bool) {
	switch id {
	case card.SkillId1:
		return card.SkillLevel1, func(level int32) { card.SkillLevel1 = level }, id != ""
	case card.SkillId2:
		return card.SkillLevel2, func(level int32) { card.SkillLevel2 = level }, id != ""
	case card.SkillId3:
		return card.SkillLevel3, func(level int32) { card.SkillLevel3 = level }, id != ""
	case card.SkillId4:
		return card.SkillLevel4, func(level int32) { card.SkillLevel4 = level }, id != ""
	default:
		return 0, nil, false
	}
}

func setBaseSkillLevel(card *ptransaction.UserCard, id string, level int32) {
	switch id {
	case card.SkillId1:
		card.BaseSkillLevel1 = level
	case card.SkillId2:
		card.BaseSkillLevel2 = level
	case card.SkillId3:
		card.BaseSkillLevel3 = level
	case card.SkillId4:
		card.BaseSkillLevel4 = level
	}
}

func nextSkillLevel(skill *pmaster.Skill, current, cardLevel int32) (*pmaster.SkillLevel, error) {
	for _, level := range skill.Levels {
		if level.Level == current+1 {
			if level.RequiredCardLevel > cardLevel {
				return nil, viber.FailedPreconditionf("card level is too low for the next skill level")
			}
			if level.RequiredItemAmount <= 0 {
				return nil, fmt.Errorf("next skill level has no positive item cost")
			}
			return level, nil
		}
	}
	return nil, viber.FailedPreconditionf("skill is already at maximum level")
}
