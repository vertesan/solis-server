package company

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *CompanyServer) BulkReceiveEnjoy(
	ctx context.Context,
	req *papi.CompanyBulkReceiveEnjoyRequest,
) (*papi.CompanyBulkReceiveEnjoyResponse, error) {
	if err := validateCompanyMissionIDs(req.GetMissionIds()); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyBulkReceiveEnjoyResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		_, beforeByCharacter, characterOrder, err := receiveCompanyMissions(
			ctx, suid, req.GetMissionIds(), true, updated,
		)
		if err != nil {
			return nil, err
		}
		state, err := findCompanyRewardStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		results := make([]*papi.CompanyBulkReceiveEnjoyResult, 0, len(characterOrder))
		for _, characterID := range characterOrder {
			character, err := opuser.FindUserCharacter(ctx, suid, characterID)
			if err != nil {
				return nil, err
			}
			levelKeys, err := receivableCompanyEnjoyLevelKeys(ctx, character, state)
			if err != nil {
				return nil, err
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
			results = append(results, &papi.CompanyBulkReceiveEnjoyResult{
				CharacterId:                      characterID,
				BeforeCharacterEnjoyPoint:        beforeByCharacter[characterID],
				EnjoyCharacterLevelRewardResults: levelResults,
			})
		}
		if err := opuser.UpsertUserCompanyRewardState(ctx, state); err != nil {
			return nil, err
		}
		response = &papi.CompanyBulkReceiveEnjoyResponse{
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
