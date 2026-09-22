package dream

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	mdream "github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"sort"
	"time"
)

func (s *DreamServer) Top(ctx context.Context, req *papi.Empty) (*papi.DreamTopResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.DreamTopResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		monthly, err := viber.ReconcileDream(ctx, suid, time.Now().UnixMilli(), updated)
		if err != nil {
			return nil, err
		}
		photos, err := opuser.FindAllUserPhotos(ctx, suid)
		if err != nil {
			return nil, err
		}
		areas, err := opuser.FindAllUserDreamAreas(ctx, suid)
		if err != nil {
			return nil, err
		}
		byID := make(map[string]*papi.DreamUserPhoto, len(areas))
		for _, area := range areas {
			byID[area.BasePhotoID] = area.Data
		}
		response = &papi.DreamTopResponse{CommonResponse: viber.NewCommonResponse(updated), SeasonInfo: &papi.DreamSeasonInfo{Id: monthly.SeasonID, Name: "イメージトレーニング", StartTime: monthly.StartTime, EndTime: monthly.EndTime - 1, GenerateAreaItemId: mdream.ManualItemID, AssetId: "event-dream-defalut"}}
		missions, err := opuser.FindAllUserMissions(ctx, suid)
		if err != nil {
			return nil, err
		}
		unlocked, err := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli()).Satisfied(mdream.UnlockConditionID)
		if err != nil {
			return nil, err
		}
		event := &papi.EventMissionInfo{Id: monthly.EventMissionID, Unlocked: unlocked}
		wanted := make(map[string]bool)
		for _, m := range monthly.Missions {
			wanted[m.GetId()] = true
		}
		for _, m := range missions {
			if wanted[m.GetMissionId()] {
				event.UserMissions = append(event.UserMissions, m)
			}
		}
		response.SeasonInfo.EventMissionInfo = event
		for plus := int64(10); plus >= 1; plus-- {
			response.SeasonInfo.RankInfos = append(response.SeasonInfo.RankInfos, &papi.DreamRankInfo{AreaRankType: penum.DreamAreaRankType_DreamAreaRankType_Splus, AreaRankPlus: plus, TopColorCode: "#FC3CB4", BottomColorCode: "#FC3CB4"})
		}
		for _, rank := range []penum.DreamAreaRankType{penum.DreamAreaRankType_DreamAreaRankType_S, penum.DreamAreaRankType_DreamAreaRankType_A, penum.DreamAreaRankType_DreamAreaRankType_B, penum.DreamAreaRankType_DreamAreaRankType_C, penum.DreamAreaRankType_DreamAreaRankType_D} {
			response.SeasonInfo.RankInfos = append(response.SeasonInfo.RankInfos, &papi.DreamRankInfo{AreaRankType: rank, TopColorCode: "#FC3CB4", BottomColorCode: "#FC3CB4"})
		}
		for _, photo := range photos {
			if photo.GetRarity() < 4 {
				continue
			}
			row := byID[photo.GetPhotoId()]
			if row == nil {
				row = &papi.DreamUserPhoto{PhotoId: photo.GetPhotoId(), IsDisableGenerateArea: !mdream.BaseEligible(photo) || photo.GetIsDreamBase()}
			}
			response.DreamUserPhotos = append(response.DreamUserPhotos, row)
		}
		sort.Slice(response.DreamUserPhotos, func(i, j int) bool {
			return response.DreamUserPhotos[i].GetPhotoId() < response.DreamUserPhotos[j].GetPhotoId()
		})
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
