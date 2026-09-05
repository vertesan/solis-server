package photo

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *PhotoServer) SaleRecipe(ctx context.Context, req *papi.PhotoSaleRecipeRequest) (*papi.PhotoSaleRecipeResponse, error) {
	ids, err := requiredPhotoRecipeIDs(req.GetPhotoRecipeIds())
	if err != nil {
		return nil, err
	}
	setting, err := viber.PhotoSetting(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if len(ids) > int(setting.GetPhotoRecipeSaleAmountLimit()) {
		return nil, viber.InvalidArgumentf("at most %d recipes can be sold", setting.GetPhotoRecipeSaleAmountLimit())
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoSaleRecipeResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		results := make([]*ptransaction.RewardResult, 0)
		amounts := make(map[string]int64, len(ids))
		order := make([]string, 0, len(ids))
		for _, id := range ids {
			if amounts[id] == 0 {
				order = append(order, id)
			}
			amounts[id]++
		}
		for _, id := range order {
			master, err := opmaster.FindPhotoRecipe(ctx, id)
			if err != nil {
				return nil, err
			}
			recipe, err := opuser.FindUserPhotoRecipe(ctx, suid, id)
			if err != nil {
				return nil, err
			}
			amount := amounts[id]
			if recipe.GetAmount() < amount {
				return nil, status.Errorf(codes.FailedPrecondition, "photo recipe %q amount is insufficient", id)
			}
			reward, err := opmaster.FindReward(ctx, master.GetSaleRewardId())
			if err != nil {
				return nil, err
			}
			recipe.Amount -= amount
			if err := opuser.UpsertUserPhotoRecipe(ctx, suid, recipe); err != nil {
				return nil, err
			}
			updated.PhotoRecipes = append(updated.PhotoRecipes, recipe)
			for range amount {
				for _, resource := range reward.GetResources() {
					granted, err := viber.GrantExchangeRewards(ctx, suid, resource, updated)
					if err != nil {
						return nil, err
					}
					results = append(results, granted...)
				}
			}
		}
		results, err = viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}
		viber.DedupePhotoUpdatedItems(updated)
		response = &papi.PhotoSaleRecipeResponse{RewardResults: results, CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func requiredPhotoRecipeIDs(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, viber.InvalidArgumentf("at least one photoRecipeId is required")
	}
	for _, value := range values {
		if value == "" {
			return nil, viber.InvalidArgumentf("photoRecipeId is required")
		}
	}
	return values, nil
}
