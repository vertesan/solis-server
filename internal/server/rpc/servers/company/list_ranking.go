package company

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *CompanyServer) ListRanking(ctx context.Context, _ *papi.Empty) (*papi.CompanyListRankingResponse, error) {
	suid := viber.GetSuid(ctx).String()
	user, public, company, err := companyRankingUser(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	totalPoint, err := companyTotalPoint(company)
	if err != nil {
		return nil, err
	}
	info := &papi.CompanyRankingInfo{
		UserId:             public.GetPublicUserId(),
		CurrentRank:        1,
		ManagerName:        user.GetName(),
		CurrentPoint:       totalPoint,
		EmblemId:           user.GetEmblemId(),
		CurrentEnjoyPoint:  company.GetEnjoyPoint(),
		CurrentTrustPoint:  company.GetTrustPoint(),
		CurrentSalaryPoint: company.GetSalaryPoint(),
	}
	return &papi.CompanyListRankingResponse{
		RankingInfos:   []*papi.CompanyRankingInfo{info},
		SelfPoint:      totalPoint,
		SelfRank:       1,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
