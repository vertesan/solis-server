package quest

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *QuestServer) ListTowerRanking(ctx context.Context, req *papi.QuestListTowerRankingRequest) (*papi.QuestListTowerRankingResponse, error) {
	if req.GetAreaId() == "" {
		return nil, viber.InvalidArgumentf("areaId is required")
	}
	area, err := opmaster.FindArea(ctx, req.GetAreaId())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if area.GetType() != penum.AreaType_AreaType_Tower && area.GetType() != penum.AreaType_AreaType_SubTower {
		return nil, viber.InvalidArgumentf("area is not a tower")
	}
	suid := viber.GetSuid(ctx).String()
	quests, err := opuser.FindAllUserQuests(ctx, suid)
	if err != nil {
		return nil, err
	}
	cleared := make(map[string]bool, len(quests))
	for _, quest := range quests {
		cleared[quest.GetQuestId()] = quest.GetLastClearedTime() > 0
	}
	// Area.questIds defines the stage progression, independently of document
	// insertion order and lexicographic IDs (which do not sort beyond 999).
	var highestQuestID string
	for _, questID := range area.GetQuestIds() {
		if cleared[questID] {
			highestQuestID = questID
		}
	}
	user, public, err := rankingUser(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.QuestListTowerRankingResponse{
		SelfHighestRank: 1,
		SelfQuestId:     highestQuestID,
		TotalQuestNum:   int32(len(area.GetQuestIds())),
		Ranks: []*papi.QuestTowerRankInfo{{
			Rank: 1, QuestId: highestQuestID, UserId: public.GetPublicUserId(),
			UserName: user.GetName(), EmblemId: user.GetEmblemId(), Cards: []*papi.LiveCardInfo{},
		}},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
