package activity

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ActivityServer) GetFanEventRanking(ctx context.Context, req *papi.ActivityGetFanEventRankingRequest) (*papi.ActivityGetFanEventRankingResponse, error) {
	if req.GetActivityFanEventId() == "" {
		return nil, status.Error(codes.InvalidArgument, "activityFanEventId is required")
	}
	suid := viber.GetSuid(ctx).String()
	info, err := opuser.FindUserFanEventInfo(ctx, suid)
	if err != nil {
		return nil, err
	}
	if findUserFanEvent(info, req.GetActivityFanEventId()) == nil {
		return nil, status.Error(codes.NotFound, "fan event not found")
	}
	ranking, err := opuser.FindUserFanEventRanking(ctx, suid, req.GetActivityFanEventId())
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		ranking = &umodel.UserFanEventRanking{SUID: suid, ActivityFanEventId: req.GetActivityFanEventId(), CharacterIds: []string{}}
	}
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, err
	}
	public, err := opuser.FindUserPublic(ctx, suid)
	if err != nil {
		return nil, err
	}
	rankInfo := &papi.FanEventRankingInfo{
		Rank:         1,
		Score:        ranking.BestScore,
		UserId:       public.GetPublicUserId(),
		Name:         user.GetName(),
		ManagerLevel: user.GetManagerLevel(),
		CharacterIds: append([]string(nil), ranking.CharacterIds...),
		EmblemId:     user.GetEmblemId(),
	}
	return &papi.ActivityGetFanEventRankingResponse{
		SelfRank:       1,
		SelfScore:      ranking.BestScore,
		RankInfos:      []*papi.FanEventRankingInfo{rankInfo},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
