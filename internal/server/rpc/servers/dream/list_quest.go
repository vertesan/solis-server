package dream

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	mdream "github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *DreamServer) ListQuest(ctx context.Context, req *papi.DreamListQuestRequest) (*papi.DreamListQuestResponse, error) {
	if req.GetBasePhotoId() == "" {
		return nil, viber.InvalidArgumentf("basePhotoId is required")
	}
	suid := viber.GetSuid(ctx).String()
	if err := viber.CheckDreamUnlocked(ctx, suid); err != nil {
		return nil, viber.GameplayError(err)
	}
	area, err := opuser.FindUserDreamArea(ctx, suid, req.GetBasePhotoId())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	photo, err := opuser.FindUserPhoto(ctx, suid, req.GetBasePhotoId())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.DreamListQuestResponse{QuestInfos: mdream.ListQuestInfos(area, photo), ClearQuestNumber: area.Data.GetClearQuestNumber(), MaxQuestNumber: area.Data.GetMaxQuestNumber(), CommonResponse: viber.NewCommonResponse(nil)}, nil
}
