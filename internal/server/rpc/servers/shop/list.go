package shop

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/protobuf/proto"
)

func (s *ShopServer) List(ctx context.Context, _ *papi.Empty) (*papi.ShopListResponse, error) {
	suid := viber.GetSuid(ctx).String()
	allowed, err := findUserShopAccess(ctx, suid)
	if err != nil {
		return nil, err
	}
	entries, err := opsolis.FindAllShopCatalogEntries(ctx)
	if err != nil {
		return nil, err
	}
	purchase, err := viber.FindUserShopPurchaseOrEmpty(ctx, suid)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Info != nil {
			allowed[entry.ShopId] = struct{}{}
		}
	}
	evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
	shops := make([]*papi.ShopInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.Info == nil {
			continue
		}
		if viber.IsShowcaseCommerceShopID(entry.ShopId) || viber.IsShowcaseShopType(entry.Info.GetType()) {
			continue
		}
		if _, ok := allowed[entry.ShopId]; !ok {
			continue
		}
		visible, err := evaluator.Satisfied(entry.Info.GetViewConditionId())
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		hasContent, err := viber.ShopCatalogHasVisibleContent(evaluator, entry)
		if err != nil {
			return nil, err
		}
		if hasContent {
			info := proto.Clone(entry.Info).(*papi.ShopInfo)
			if entry.LoginBonusPackageItem != nil {
				info, _, err = viber.RenderLoginBonusShop(entry, purchase)
				if err != nil {
					return nil, err
				}
			}
			if info.NoticeInfo == nil {
				info.NoticeInfo = viber.NewDummyNoticeInfo()
			}
			shops = append(shops, info)
		}
	}
	return &papi.ShopListResponse{
		Shops:          shops,
		DokanInfos:     []*papi.DokanInfo{},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
