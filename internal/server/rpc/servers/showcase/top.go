package showcase

import (
	"context"
	"errors"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

func (s *ShowcaseServer) Top(ctx context.Context, _ *papi.Empty) (*papi.ShowcaseTopResponse, error) {
	suid := viber.GetSuid(ctx).String()
	catalog, err := viber.BuildShowcaseCommerceCatalog(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	shops := make([]*papi.ShopInfo, 0, len(catalog.Shops()))
	for _, entry := range catalog.Shops() {
		if entry != nil && entry.Info != nil {
			shops = append(shops, proto.Clone(entry.Info).(*papi.ShopInfo))
		}
	}
	unlocked, err := opuser.FindUserUnlockedShowcaseNumber(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	unlockedNumbers := make(map[int32]struct{}, len(unlocked.Numbers))
	for _, number := range unlocked.Numbers {
		unlockedNumbers[number] = struct{}{}
	}

	showcaseInfos := make([]*papi.ShowcaseSelfInfo, 0, showcaseSlotCount)
	for number := int32(1); number <= showcaseSlotCount; number++ {
		layout, err := findShowcaseLayoutOrEmpty(ctx, suid, number)
		if err != nil {
			return nil, viber.GameplayError(err)
		}
		_, isUnlocked := unlockedNumbers[number]
		showcaseInfos = append(showcaseInfos, &papi.ShowcaseSelfInfo{
			Number:            number,
			DefaultName:       showcaseDefaultName(number),
			UnlockConditionId: showcaseSlotConditionID(number),
			IsUnlocked:        isUnlocked,
			LayoutInfo:        layout,
		})
	}

	mysetInfos := make([]*papi.ShowcaseMysetInfo, 0, showcaseMysetSlotCount)
	for number := int32(1); number <= showcaseMysetSlotCount; number++ {
		mysetInfos = append(mysetInfos, &papi.ShowcaseMysetInfo{
			Number:      number,
			DefaultName: showcaseDefaultMysetName(number),
		})
	}

	frames, err := opmaster.FindAllShowcaseFrames(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	sort.Slice(frames, func(i, j int) bool {
		if frames[i].GetOrder() != frames[j].GetOrder() {
			return frames[i].GetOrder() < frames[j].GetOrder()
		}
		return frames[i].GetId() < frames[j].GetId()
	})
	frameInfos := make([]*papi.ShowcaseFrameInfo, 0, len(frames))
	for _, frame := range frames {
		owned, err := userOwnsShowcaseFrame(ctx, suid, frame.GetId())
		if err != nil {
			return nil, viber.GameplayError(err)
		}
		frameInfos = append(frameInfos, &papi.ShowcaseFrameInfo{Id: frame.GetId(), IsUnlocked: owned})
	}

	musics, err := opmaster.FindAllShowcaseMusics(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	sort.Slice(musics, func(i, j int) bool {
		if musics[i].GetOrder() != musics[j].GetOrder() {
			return musics[i].GetOrder() < musics[j].GetOrder()
		}
		return musics[i].GetMusicId() < musics[j].GetMusicId()
	})
	musicIDs := make([]string, 0, len(musics))
	for _, music := range musics {
		if music.GetMusicId() != "" {
			musicIDs = append(musicIDs, music.GetMusicId())
		}
	}

	return &papi.ShowcaseTopResponse{
		Shops:            shops,
		DokanInfos:       []*papi.DokanInfo{},
		ShowcaseInfos:    showcaseInfos,
		TotalLikedCount:  0,
		MysetInfos:       mysetInfos,
		FrameInfos:       frameInfos,
		TopDokanInfos:    []*papi.DokanInfo{},
		ShowcaseMusicIds: musicIDs,
		CommonResponse:   viber.NewCommonResponse(nil),
	}, nil
}

func findShowcaseLayoutOrEmpty(ctx context.Context, suid string, number int32) (*papi.ShowcaseLayoutInfo, error) {
	layout, err := opuser.FindUserShowcaseLayout(ctx, suid, number)
	if errors.Is(err, mongo.ErrNoDocuments) || layout == nil {
		return emptyShowcaseLayout(), nil
	}
	if err != nil {
		return nil, err
	}
	return layout, nil
}

func emptyShowcaseLayout() *papi.ShowcaseLayoutInfo {
	return &papi.ShowcaseLayoutInfo{
		ShowcaseToyAssetInfos:       []*papi.ShowcaseToyAssetInfo{},
		ShowcaseToyArrangementInfos: []*papi.ShowcaseToyArrangementInfo{},
	}
}

func showcaseSlotConditionID(number int32) string {
	if number == showcaseSlotCount {
		return showcaseFifthSlotConditionID
	}
	return ""
}
