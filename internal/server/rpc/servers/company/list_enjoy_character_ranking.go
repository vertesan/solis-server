package company

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *CompanyServer) ListEnjoyCharacterRanking(
	ctx context.Context,
	req *papi.CompanyListEnjoyCharacterRankingRequest,
) (*papi.CompanyListEnjoyCharacterRankingResponse, error) {
	if req.GetCharacterId() == "" {
		return nil, viber.InvalidArgumentf("characterId is required")
	}
	suid := viber.GetSuid(ctx).String()
	character, err := requireOwnedCompanyCharacter(ctx, suid, req.GetCharacterId())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	user, public, _, err := companyRankingUser(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	info := &papi.CompanyEnjoyCharacterRankingInfo{
		UserId:            public.GetPublicUserId(),
		CurrentRank:       1,
		ManagerName:       user.GetName(),
		CurrentEnjoyPoint: character.GetCompanyEnjoyPoint(),
		EmblemId:          user.GetEmblemId(),
	}
	return &papi.CompanyListEnjoyCharacterRankingResponse{
		RankingInfos:   []*papi.CompanyEnjoyCharacterRankingInfo{info},
		SelfPoint:      character.GetCompanyEnjoyPoint(),
		SelfRank:       1,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
