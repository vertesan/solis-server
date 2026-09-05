package company

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *CompanyServer) BulkReceiveEnjoyCharacterLevelRewards(
	ctx context.Context,
	_ *papi.Empty,
) (*papi.CompanyBulkReceiveEnjoyCharacterLevelRewardsResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyBulkReceiveEnjoyCharacterLevelRewardsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		state, err := findCompanyRewardStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		characters, err := opuser.FindAllUserCharacters(ctx, suid)
		if err != nil {
			return nil, err
		}
		sort.SliceStable(characters, func(i, j int) bool {
			return characters[i].GetCharacterId() < characters[j].GetCharacterId()
		})
		updated := &papi.UpdatedData{}
		results := make([]*papi.CompanyBulkReceiveEnjoyResult, 0)
		for _, character := range characters {
			levelKeys, err := receivableCompanyEnjoyLevelKeys(ctx, character, state)
			if err != nil {
				return nil, err
			}
			if len(levelKeys) == 0 {
				continue
			}
			levelResults, err := claimCompanyEnjoyLevelRewards(
				ctx, suid, character, levelKeys, state, false, updated,
			)
			if err != nil {
				return nil, err
			}
			levelResults, err = viber.AggregateRewardResults(levelResults)
			if err != nil {
				return nil, err
			}
			putUpdatedCompanyCharacter(updated, character)
			results = append(results, &papi.CompanyBulkReceiveEnjoyResult{
				CharacterId:                      character.GetCharacterId(),
				BeforeCharacterEnjoyPoint:        character.GetCompanyEnjoyPoint(),
				EnjoyCharacterLevelRewardResults: levelResults,
			})
		}
		if err := opuser.UpsertUserCompanyRewardState(ctx, state); err != nil {
			return nil, err
		}
		response = &papi.CompanyBulkReceiveEnjoyCharacterLevelRewardsResponse{
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
