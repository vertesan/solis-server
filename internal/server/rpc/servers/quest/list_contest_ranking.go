package quest

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (s *QuestServer) ListContestRanking(ctx context.Context, req *papi.QuestListContestRankingRequest) (*papi.QuestListContestRankingResponse, error) {
	if req.GetQuestId() == "" {
		return nil, viber.InvalidArgumentf("questId is required")
	}
	master, err := opmaster.FindQuest(ctx, req.GetQuestId())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	area, err := opmaster.FindArea(ctx, master.GetAreaId())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if area.GetType() != penum.AreaType_AreaType_Contest {
		return nil, viber.InvalidArgumentf("quest is not a contest")
	}
	suid := viber.GetSuid(ctx).String()
	quest, err := opuser.FindUserQuest(ctx, suid, master.GetId())
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	score := quest.GetHighestScore()
	user, public, err := rankingUser(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	rank := &papi.QuestContestRankInfo{
		Rank: 1, Score: score, UserId: public.GetPublicUserId(),
		UserName: user.GetName(), EmblemId: user.GetEmblemId(), Cards: []*papi.LiveCardInfo{},
	}
	replay, err := opuser.FindQuestLiveReplayInfo(ctx, suid, master.GetId(), opuser.QuestLiveReplayBest)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	// God-created high scores have no replay. Also do not advertise a historical
	// deck as belonging to a score that has since been replaced independently.
	if replay != nil && replay.Score == score {
		rank.DeckName = replay.DeckName
		rank.Cards = replay.Cards
		rank.IsLiveResultDetailAvailable = true
	}
	return &papi.QuestListContestRankingResponse{
		SelfHighestRank: 1,
		SelfScore:       score,
		Ranks:           []*papi.QuestContestRankInfo{rank},
		CommonResponse:  viber.NewCommonResponse(nil),
	}, nil
}
