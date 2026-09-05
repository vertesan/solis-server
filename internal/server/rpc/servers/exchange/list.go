package exchange

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

func (s *ExchangeServer) List(ctx context.Context, _ *papi.Empty) (*papi.ExchangeListResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ExchangeListResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
		purchase, err := findExchangePurchaseOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		catalog, err := opsolis.FindAllExchangeBooths(ctx)
		if err != nil {
			return nil, err
		}

		userBooths, err := opuser.FindUserExchangeBooth(ctx, suid)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		if userBooths == nil {
			userBooths = &umodel.UserExchangeBooth{SUID: suid}
		}
		allowed := viber.StringSet(userBooths.BoothIds)
		for _, definition := range catalog {
			if _, ok := allowed[definition.GetId()]; !ok {
				allowed[definition.GetId()] = struct{}{}
				userBooths.BoothIds = append(userBooths.BoothIds, definition.GetId())
			}
		}
		if err := opuser.UpsertUserExchangeBooth(ctx, userBooths); err != nil {
			return nil, err
		}
		evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now)
		booths := make([]*papi.ExchangeBooth, 0, len(catalog))
		for _, definition := range catalog {
			if _, ok := allowed[definition.Id]; !ok {
				continue
			}
			if definition.EndTime > 0 && definition.EndTime <= now {
				continue
			}
			booth, visible, err := renderExchangeBooth(evaluator, definition, purchase)
			if err != nil {
				return nil, err
			}
			if visible {
				booths = append(booths, booth)
			}
		}
		response = &papi.ExchangeListResponse{Booths: booths, CommonResponse: viber.NewCommonResponse(nil)}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func renderExchangeBooth(
	evaluator *viber.CommerceConditionEvaluator,
	definition *papi.ExchangeBooth,
	purchase *umodel.UserExchangePurchase,
) (*papi.ExchangeBooth, bool, error) {
	visible, err := evaluator.Satisfied(definition.GetViewConditionId())
	if err != nil || !visible {
		return nil, visible, err
	}
	unlocked, err := evaluator.Satisfied(definition.GetUnlockConditionId())
	if err != nil {
		return nil, false, err
	}

	booth := proto.Clone(definition).(*papi.ExchangeBooth)
	booth.Unlocked = unlocked
	applyExchangeStock(booth, purchase)
	items := make([]*papi.ExchangeItem, 0, len(booth.Exchanges))
	for _, item := range booth.Exchanges {
		itemVisible, err := evaluator.Satisfied(item.GetViewConditionId())
		if err != nil {
			return nil, false, err
		}
		if !itemVisible {
			continue
		}
		itemUnlocked, err := evaluator.Satisfied(item.GetUnlockConditionId())
		if err != nil {
			return nil, false, err
		}
		item.Unlocked = itemUnlocked
		items = append(items, item)
	}
	booth.Exchanges = items
	return booth, true, nil
}
