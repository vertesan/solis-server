package company

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *CompanyServer) ReceiveEnjoyCharacterLevelRewards(
	ctx context.Context,
	req *papi.CompanyReceiveEnjoyCharacterLevelRewardsRequest,
) (*papi.CompanyReceiveEnjoyCharacterLevelRewardsResponse, error) {
	if req.GetCharacterId() == "" || len(req.GetLevelKeys()) == 0 {
		return nil, viber.InvalidArgumentf("characterId and levelKeys are required")
	}
	if err := validateUniqueLevelKeys(req.GetLevelKeys()); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyReceiveEnjoyCharacterLevelRewardsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		character, err := requireOwnedCompanyCharacter(ctx, suid, req.GetCharacterId())
		if err != nil {
			return nil, err
		}
		state, err := findCompanyRewardStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		results, err := claimCompanyEnjoyLevelRewards(
			ctx, suid, character, req.GetLevelKeys(), state, true, updated,
		)
		if err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserCompanyRewardState(ctx, state); err != nil {
			return nil, err
		}
		putUpdatedCompanyCharacter(updated, character)
		results, err = viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}
		response = &papi.CompanyReceiveEnjoyCharacterLevelRewardsResponse{
			Results:        results,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
