package card

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (s *CardServer) SetSupport(ctx context.Context, req *papi.CardSetSupportRequest) (*papi.CardSetSupportResponse, error) {
	if req.GetNumber() <= 0 || req.GetCardId() == "" {
		return nil, viber.InvalidArgumentf("positive number and cardId are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CardSetSupportResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		card, err := opuser.FindUserCard(ctx, suid, req.CardId)
		if err != nil {
			return nil, err
		}
		if card.Supported {
			return nil, viber.FailedPreconditionf("card is already assigned as support")
		}
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		if req.Number > user.CardSupportMaxNumber {
			return nil, viber.InvalidArgumentf("card support number must be between 1 and %d", user.CardSupportMaxNumber)
		}
		updated := &papi.UpdatedData{}
		trainers, err := viber.ReconcileCardSupportLevel(ctx, suid, user, updated)
		if err != nil {
			return nil, err
		}
		for index, trainer := range trainers {
			// Equal-level cards can take a tied trainer's place. Do not make
			// arbitrary card ID tie-breaking prevent an otherwise valid choice.
			if trainer.CardId == card.CardId && index < 5 &&
				(len(trainers) < 6 || trainers[5].BaseLevel < card.BaseLevel) {
				return nil, viber.FailedPreconditionf("a trainer cannot receive level assistance")
			}
		}
		if card.BaseLevel > user.CardSupportLevel {
			return nil, viber.FailedPreconditionf("level assistance cannot lower a card's trained level")
		}
		if err := opuser.UpsertUser(ctx, suid, user); err != nil {
			return nil, err
		}
		settingID, err := config.GetGameSettingId()
		if err != nil {
			return nil, err
		}
		setting, err := opmaster.FindSetting(ctx, settingID)
		if err != nil {
			return nil, err
		}
		if user.CardSupportLevel <= 0 || user.CardSupportLevel > setting.CardSupportLevelLimit {
			return nil, viber.FailedPreconditionf("user card support level is outside the master limit")
		}
		maxRelease, err := opmaster.FindMaxCardSupportRelease(ctx)
		if err != nil {
			return nil, err
		}
		if req.Number > maxRelease.Count+3 {
			return nil, viber.InvalidArgumentf("card support number exceeds the master slot limit")
		}
		supports, err := opuser.FindAllUserCardSupports(ctx, suid)
		if err != nil {
			return nil, err
		}
		for _, support := range supports {
			if support.Number == req.Number && support.CardId != "" {
				return nil, viber.FailedPreconditionf("card support slot is already occupied")
			}
			if support.CardId == req.CardId {
				return nil, viber.FailedPreconditionf("card is already assigned as support")
			}
		}
		masterCard, err := opmaster.FindCard(ctx, card.CardId)
		if err != nil {
			return nil, err
		}
		supportLevel := user.CardSupportLevel
		if err := usercard.ApplySupportLevel(ctx, masterCard, card, supportLevel); err != nil {
			return nil, err
		}
		support := &ptransaction.UserCardSupport{
			Number: req.Number, CardId: req.CardId,
			RemovableTime: time.Now().Add(time.Duration(setting.CardSupportRemovableMinutes) * time.Minute).UnixMilli(),
		}
		card.Supported = true
		if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserCardSupport(ctx, suid, support); err != nil {
			return nil, err
		}
		stories, _, err := viber.UpdateCardStoryUnlocks(ctx, suid, masterCard, supportLevel, card.EvolutionLevel)
		if err != nil {
			return nil, err
		}
		missions, err := viber.ReachCharacterCardTrainingMissions(ctx, suid, masterCard.CharacterId)
		if err != nil {
			return nil, err
		}
		updated.Cards = append(updated.Cards, card)
		updated.CardSupports = []*ptransaction.UserCardSupport{support}
		updated.Stories = append(updated.Stories, stories...)
		viber.MergeCardTrainingMissions(updated, missions)
		response = &papi.CardSetSupportResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, viber.FailedPreconditionf("card support slot or card is already assigned")
		}
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
