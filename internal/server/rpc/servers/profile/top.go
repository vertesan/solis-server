package profile

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ProfileServer) Top(ctx context.Context, _ *papi.Empty) (*papi.ProfileTopResponse, error) {
	gradeID, err := profileHierarchyGrade(ctx, viber.GetSuid(ctx).String())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.ProfileTopResponse{
		HierarchyCurrentDetailGradeId: gradeID,
		HierarchyCurrentRank:          viber.HierarchyPrivateServerRank,
		FriendProfiles:                []*papi.ProfileInfo{},
		CommonResponse:                viber.NewCommonResponse(nil),
	}, nil
}
