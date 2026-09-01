package servers

import (
	"context"
	"errors"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	showcaseDefaultFrameID       = "frame-001"
	showcaseEventName            = "グッズケース"
	showcaseSlotCount            = int32(5)
	showcaseMysetSlotCount       = int32(5)
	showcaseMaximumHashtagCount  = 3
	showcaseFifthSlotConditionID = "cd-showcase-005-view"
)

var showcaseJST = time.FixedZone("JST", 9*60*60)

type ShowcaseServer struct {
	papi.UnimplementedShowcaseServer
}

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

func (s *ShowcaseServer) Save(ctx context.Context, req *papi.ShowcaseSaveRequest) (*papi.ShowcaseSaveResponse, error) {
	if req.GetNumber() <= 0 || req.GetFrameId() == "" || req.GetAssetId() == "" || req.GetLayoutInfo() == nil {
		return nil, viber.InvalidArgumentf("number, frameId, assetId, and layoutInfo are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ShowcaseSaveResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := requireUnlockedShowcaseNumber(ctx, suid, req.GetNumber()); err != nil {
			return nil, err
		}
		frame, err := requireOwnedShowcaseFrame(ctx, suid, req.GetFrameId())
		if err != nil {
			return nil, err
		}
		toys, err := validateShowcaseLayout(ctx, suid, frame, req.GetLayoutInfo())
		if err != nil {
			return nil, err
		}
		showcase, err := findShowcaseOrDefault(ctx, suid, req.GetNumber())
		if err != nil {
			return nil, err
		}
		showcase.ShowcaseFrameId = frame.GetId()
		showcase.ThumbnailAssetId = req.GetAssetId()
		showcase.HashTagIds, err = deriveShowcaseHashtags(ctx, req.GetLayoutInfo(), toys)
		if err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserShowcase(ctx, suid, showcase); err != nil {
			return nil, err
		}
		layout := proto.Clone(req.GetLayoutInfo()).(*papi.ShowcaseLayoutInfo)
		if err := opuser.UpsertUserShowcaseLayout(ctx, suid, req.GetNumber(), layout); err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{Showcases: []*ptransaction.UserShowcase{showcase}}
		saveMissions, err := viber.AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_ShowcaseSaveAchievement, 1,
		)
		if err != nil {
			return nil, err
		}
		hashtagMissions, err := viber.AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_ShowcaseSpecifiedHashtagIdSaveCount, 1,
			showcase.GetHashTagIds()...,
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = viber.AppendUniquePhotoMissions(saveMissions, hashtagMissions)
		if err := consumeShowcaseThumbnailImageID(ctx, suid, req.GetAssetId(), updated); err != nil {
			return nil, err
		}
		response = &papi.ShowcaseSaveResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *ShowcaseServer) SaveDetail(ctx context.Context, req *papi.ShowcaseSaveDetailRequest) (*papi.ShowcaseSaveDetailResponse, error) {
	if req.GetNumber() <= 0 || strings.TrimSpace(req.GetName()) == "" {
		return nil, viber.InvalidArgumentf("number and name are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ShowcaseSaveDetailResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := requireUnlockedShowcaseNumber(ctx, suid, req.GetNumber()); err != nil {
			return nil, err
		}
		setting, err := opmaster.FindDefaultShowcaseSetting(ctx)
		if err != nil {
			return nil, err
		}
		if utf8.RuneCountInString(req.GetName()) > int(setting.GetShowcaseDetailNameLimit()) {
			return nil, viber.InvalidArgumentf("showcase name exceeds the master-data limit")
		}
		if utf8.RuneCountInString(req.GetComment()) > int(setting.GetShowcaseDetailDescriptionLimit()) {
			return nil, viber.InvalidArgumentf("showcase comment exceeds the master-data limit")
		}
		showcase, err := opuser.FindUserShowcase(ctx, suid, req.GetNumber())
		if err != nil {
			return nil, err
		}
		if req.GetMusicId() != "" {
			if _, err := opmaster.FindShowcaseMusic(ctx, req.GetMusicId()); err != nil {
				return nil, err
			}
			if _, err := opuser.FindUserShowcaseMusic(ctx, suid, req.GetMusicId()); err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					return nil, status.Error(codes.FailedPrecondition, "showcase BGM is not owned")
				}
				return nil, err
			}
		}
		showcase.Name = req.GetName()
		showcase.Comment = req.GetComment()
		showcase.MusicId = req.GetMusicId()
		showcase.IsOpen = req.GetIsOpen()
		if err := opuser.UpsertUserShowcase(ctx, suid, showcase); err != nil {
			return nil, err
		}
		response = &papi.ShowcaseSaveDetailResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			Showcases: []*ptransaction.UserShowcase{showcase},
		})}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *ShowcaseServer) SaveMyset(ctx context.Context, req *papi.ShowcaseSaveMysetRequest) (*papi.ShowcaseSaveMysetResponse, error) {
	if !validShowcaseMysetNumber(req.GetNumber()) || req.GetFrameId() == "" || req.GetLayoutInfo() == nil {
		return nil, viber.InvalidArgumentf("valid number, frameId, and layoutInfo are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ShowcaseSaveMysetResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		frame, err := requireOwnedShowcaseFrame(ctx, suid, req.GetFrameId())
		if err != nil {
			return nil, err
		}
		if _, err := validateShowcaseLayout(ctx, suid, frame, req.GetLayoutInfo()); err != nil {
			return nil, err
		}
		myset, err := findShowcaseMysetOrDefault(ctx, suid, req.GetNumber())
		if err != nil {
			return nil, err
		}
		myset.ShowcaseFrameId = frame.GetId()
		myset.IsSave = true
		if err := opuser.UpsertUserShowcaseMyset(ctx, suid, myset); err != nil {
			return nil, err
		}
		layout := proto.Clone(req.GetLayoutInfo()).(*papi.ShowcaseLayoutInfo)
		if err := opuser.UpsertUserShowcaseMysetLayout(ctx, suid, req.GetNumber(), layout); err != nil {
			return nil, err
		}
		response = &papi.ShowcaseSaveMysetResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			ShowcaseMysets: []*ptransaction.UserShowcaseMyset{myset},
		})}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *ShowcaseServer) DeleteMyset(ctx context.Context, req *papi.ShowcaseDeleteMysetRequest) (*papi.ShowcaseDeleteMysetResponse, error) {
	if !validShowcaseMysetNumber(req.GetNumber()) {
		return nil, viber.InvalidArgumentf("valid number is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ShowcaseDeleteMysetResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := opuser.DeleteUserShowcaseMyset(ctx, suid, req.GetNumber()); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserShowcaseMysetLayout(ctx, suid, req.GetNumber()); err != nil {
			return nil, err
		}
		common := viber.NewCommonResponse(nil)
		common.DeletedData.ShowcaseMysets = []*ptransaction.UserShowcaseMyset{{Number: req.GetNumber()}}
		response = &papi.ShowcaseDeleteMysetResponse{CommonResponse: common}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *ShowcaseServer) ApplyMyset(ctx context.Context, req *papi.ShowcaseApplyMysetRequest) (*papi.ShowcaseApplyMysetResponse, error) {
	if !validShowcaseMysetNumber(req.GetNumber()) {
		return nil, viber.InvalidArgumentf("valid number is required")
	}
	suid := viber.GetSuid(ctx).String()
	myset, err := opuser.FindUserShowcaseMyset(ctx, suid, req.GetNumber())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if !myset.GetIsSave() || myset.GetShowcaseFrameId() == "" {
		return nil, status.Error(codes.FailedPrecondition, "showcase myset is not saved")
	}
	layout, err := opuser.FindUserShowcaseMysetLayout(ctx, suid, req.GetNumber())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.ShowcaseApplyMysetResponse{
		FrameId:        myset.GetShowcaseFrameId(),
		LayoutInfo:     proto.Clone(layout).(*papi.ShowcaseLayoutInfo),
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *ShowcaseServer) RenameMyset(ctx context.Context, req *papi.ShowcaseRenameMysetRequest) (*papi.ShowcaseRenameMysetResponse, error) {
	if !validShowcaseMysetNumber(req.GetNumber()) || strings.TrimSpace(req.GetName()) == "" {
		return nil, viber.InvalidArgumentf("valid number and name are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ShowcaseRenameMysetResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		setting, err := opmaster.FindDefaultShowcaseSetting(ctx)
		if err != nil {
			return nil, err
		}
		if utf8.RuneCountInString(req.GetName()) > int(setting.GetShowcaseMysetNameLimit()) {
			return nil, viber.InvalidArgumentf("showcase myset name exceeds the master-data limit")
		}
		myset, err := findShowcaseMysetOrDefault(ctx, suid, req.GetNumber())
		if err != nil {
			return nil, err
		}
		myset.Name = req.GetName()
		if err := opuser.UpsertUserShowcaseMyset(ctx, suid, myset); err != nil {
			return nil, err
		}
		response = &papi.ShowcaseRenameMysetResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			ShowcaseMysets: []*ptransaction.UserShowcaseMyset{myset},
		})}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *ShowcaseServer) CheckThumbnailShooting(ctx context.Context, req *papi.ShowcaseCheckThumbnailShootingRequest) (*papi.ShowcaseCheckThumbnailShootingResponse, error) {
	if req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("number is required")
	}
	suid := viber.GetSuid(ctx).String()
	if err := requireUnlockedShowcaseNumber(ctx, suid, req.GetNumber()); err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.ShowcaseCheckThumbnailShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}

func (s *ShowcaseServer) Like(ctx context.Context, req *papi.ShowcaseLikeRequest) (*papi.ShowcaseLikeResponse, error) {
	if req.GetUserId() == "" || req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("userId and number are required")
	}
	return &papi.ShowcaseLikeResponse{
		Rewards:        []*ptransaction.Reward{},
		IsAlreadyLike:  true,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *ShowcaseServer) Get(ctx context.Context, req *papi.ShowcaseGetRequest) (*papi.ShowcaseGetResponse, error) {
	if req.GetUserId() == "" {
		return nil, viber.InvalidArgumentf("userId is required")
	}
	return &papi.ShowcaseGetResponse{
		ShowcaseInfos:  []*papi.ShowcaseInfo{},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *ShowcaseServer) GalleryTop(ctx context.Context, _ *papi.Empty) (*papi.ShowcaseGalleryTopResponse, error) {
	return &papi.ShowcaseGalleryTopResponse{
		ShowcaseInfos:  []*papi.ShowcaseInfo{},
		TermInfo:       currentShowcaseTermInfo(),
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *ShowcaseServer) SearchFriendRecommendList(ctx context.Context, _ *papi.Empty) (*papi.ShowcaseSearchFriendRecommendListResponse, error) {
	return &papi.ShowcaseSearchFriendRecommendListResponse{
		ShowcaseInfos:  []*papi.ShowcaseInfo{},
		TermInfo:       currentShowcaseTermInfo(),
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *ShowcaseServer) SearchHashtagList(ctx context.Context, req *papi.ShowcaseSearchHashtagListRequest) (*papi.ShowcaseSearchHashtagListResponse, error) {
	if req.GetHashtagId() == "" {
		return nil, viber.InvalidArgumentf("hashtagId is required")
	}
	hashtags, err := opmaster.FindAllShowcaseHashtags(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if !slices.ContainsFunc(hashtags, func(value *pmaster.ShowcaseHashtag) bool {
		return value.GetId() == req.GetHashtagId()
	}) {
		return nil, status.Error(codes.NotFound, "showcase hashtag was not found")
	}
	return &papi.ShowcaseSearchHashtagListResponse{
		ShowcaseInfos:          []*papi.ShowcaseInfo{},
		TermInfo:               currentShowcaseTermInfo(),
		IsEndHashtagSearchList: true,
		IsAlwaysEmpty:          true,
		CommonResponse:         viber.NewCommonResponse(nil),
	}, nil
}

func (s *ShowcaseServer) Display(ctx context.Context, req *papi.ShowcaseDisplayRequest) (*papi.ShowcaseDisplayResponse, error) {
	if req.GetUserId() == "" || req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("userId and number are required")
	}
	return &papi.ShowcaseDisplayResponse{
		IsViewable:     false,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *ShowcaseServer) CheckShooting(ctx context.Context, req *papi.ShowcaseCheckShootingRequest) (*papi.ShowcaseCheckShootingResponse, error) {
	if req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("number is required")
	}
	suid := viber.GetSuid(ctx).String()
	if _, _, err := findSavedShowcase(ctx, suid, req.GetNumber()); err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.ShowcaseCheckShootingResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}

func (s *ShowcaseServer) CreatePhoto(ctx context.Context, req *papi.ShowcaseCreatePhotoRequest) (*papi.ShowcaseCreatePhotoResponse, error) {
	if req.GetNumber() <= 0 || len(req.GetCreatePhotoParams()) == 0 {
		return nil, viber.InvalidArgumentf("number and at least one createPhotoParam are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ShowcaseCreatePhotoResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		showcase, layout, err := findSavedShowcase(ctx, suid, req.GetNumber())
		if err != nil {
			return nil, err
		}
		candidates, err := showcasePhotoCharacters(ctx, layout)
		if err != nil {
			return nil, err
		}
		params := make([]photoCreateParam, 0, len(req.GetCreatePhotoParams()))
		for _, param := range req.GetCreatePhotoParams() {
			if param == nil {
				return nil, viber.InvalidArgumentf("createPhotoParam is required")
			}
			if param.GetAssetId() == "" || param.GetItemId() == "" {
				return nil, viber.InvalidArgumentf("assetId and itemId are required")
			}
			film, err := opmaster.FindItem(ctx, param.GetItemId())
			if err != nil {
				return nil, err
			}
			if film.GetIsPhotoShootingCharacterOnly() {
				return nil, status.Error(codes.FailedPrecondition, "character-only shooting film cannot be used for showcase shooting")
			}
			params = append(params, photoCreateParam{
				imageID:          param.GetAssetId(),
				itemID:           param.GetItemId(),
				focusCharacterID: param.GetMainCharacterId(),
			})
		}
		placeName := showcase.GetName()
		if placeName == "" {
			placeName = showcaseDefaultName(req.GetNumber())
		}
		result, err := createUserPhotoShootings(ctx, suid, photoShootingScene{
			placeName:                   placeName,
			eventName:                   showcaseEventName,
			photoNamePrefix:             showcaseEventName,
			candidateCharacterIDs:       candidates,
			showcaseShooting:            true,
			reliabilityOncePerCharacter: true,
		}, params, req.GetManualCount())
		if err != nil {
			return nil, err
		}
		response = &papi.ShowcaseCreatePhotoResponse{
			Photos:         result.photos,
			IsGift:         result.isGift,
			CommonResponse: viber.NewCommonResponse(result.updated),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func findShowcaseOrDefault(ctx context.Context, suid string, number int32) (*ptransaction.UserShowcase, error) {
	showcase, err := opuser.FindUserShowcase(ctx, suid, number)
	if err == nil {
		return showcase, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	return &ptransaction.UserShowcase{
		Number: number,
		Name:   showcaseDefaultName(number),
		IsOpen: true,
	}, nil
}

func findShowcaseMysetOrDefault(ctx context.Context, suid string, number int32) (*ptransaction.UserShowcaseMyset, error) {
	myset, err := opuser.FindUserShowcaseMyset(ctx, suid, number)
	if err == nil {
		return myset, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	return &ptransaction.UserShowcaseMyset{Number: number, Name: showcaseDefaultMysetName(number)}, nil
}

func findSavedShowcase(ctx context.Context, suid string, number int32) (*ptransaction.UserShowcase, *papi.ShowcaseLayoutInfo, error) {
	if err := requireUnlockedShowcaseNumber(ctx, suid, number); err != nil {
		return nil, nil, err
	}
	showcase, err := opuser.FindUserShowcase(ctx, suid, number)
	if err != nil {
		return nil, nil, err
	}
	layout, err := opuser.FindUserShowcaseLayout(ctx, suid, number)
	if err != nil {
		return nil, nil, err
	}
	if showcase.GetShowcaseFrameId() == "" || layout == nil {
		return nil, nil, status.Error(codes.FailedPrecondition, "showcase is not saved")
	}
	return showcase, layout, nil
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

func requireUnlockedShowcaseNumber(ctx context.Context, suid string, number int32) error {
	if number <= 0 || number > showcaseSlotCount {
		return viber.InvalidArgumentf("showcase number must be between 1 and %d", showcaseSlotCount)
	}
	unlocked, err := opuser.FindUserUnlockedShowcaseNumber(ctx, suid)
	if err != nil {
		return err
	}
	if !slices.Contains(unlocked.Numbers, number) {
		return status.Error(codes.FailedPrecondition, "showcase slot is locked")
	}
	return nil
}

func requireOwnedShowcaseFrame(ctx context.Context, suid, frameID string) (*pmaster.ShowcaseFrame, error) {
	frame, err := opmaster.FindShowcaseFrame(ctx, frameID)
	if err != nil {
		return nil, err
	}
	owned, err := userOwnsShowcaseFrame(ctx, suid, frameID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, status.Error(codes.FailedPrecondition, "showcase frame is not owned")
	}
	return frame, nil
}

func userOwnsShowcaseFrame(ctx context.Context, suid, frameID string) (bool, error) {
	if frameID == showcaseDefaultFrameID {
		return true, nil
	}
	items, err := opuser.FindUserItems(ctx, suid, "item-showcase_"+frameID)
	if err != nil {
		return false, err
	}
	now := time.Now().UnixMilli()
	for _, item := range items {
		if item.GetAmount() > 0 && (item.GetExpiredTime() == 0 || item.GetExpiredTime() > now) {
			return true, nil
		}
	}
	return false, nil
}

func validateShowcaseLayout(
	ctx context.Context,
	suid string,
	frame *pmaster.ShowcaseFrame,
	layout *papi.ShowcaseLayoutInfo,
) (map[string]*pmaster.ShowcaseToy, error) {
	if layout == nil {
		return nil, viber.InvalidArgumentf("layoutInfo is required")
	}
	if len(layout.GetShowcaseToyArrangementInfos()) > int(frame.GetShowcaseToyPutCountLimit()) {
		return nil, status.Error(codes.FailedPrecondition, "showcase frame placement limit exceeded")
	}
	owned, err := opuser.FindAllUserShowcaseToys(ctx, suid)
	if err != nil {
		return nil, err
	}
	ownedAmounts := make(map[string]int64, len(owned))
	for _, toy := range owned {
		ownedAmounts[toy.GetShowcaseToyId()] = toy.GetAmount()
	}

	assets := make(map[int32]*papi.ShowcaseToyAssetInfo, len(layout.GetShowcaseToyAssetInfos()))
	toys := make(map[string]*pmaster.ShowcaseToy, len(layout.GetShowcaseToyAssetInfos()))
	requestedAmounts := make(map[string]int64, len(layout.GetShowcaseToyAssetInfos()))
	for _, asset := range layout.GetShowcaseToyAssetInfos() {
		if asset == nil || asset.GetUniqueNumber() <= 0 || asset.GetShowcaseToyId() == "" || asset.GetCount() <= 0 {
			return nil, viber.InvalidArgumentf("showcase toy asset values are invalid")
		}
		if _, duplicate := assets[asset.GetUniqueNumber()]; duplicate {
			return nil, viber.InvalidArgumentf("showcase toy asset uniqueNumber values must be unique")
		}
		masterToy, err := opmaster.FindShowcaseToy(ctx, asset.GetShowcaseToyId())
		if err != nil {
			return nil, err
		}
		requestedAmounts[asset.GetShowcaseToyId()] += int64(asset.GetCount())
		if requestedAmounts[asset.GetShowcaseToyId()] > ownedAmounts[asset.GetShowcaseToyId()] {
			return nil, status.Errorf(codes.FailedPrecondition, "insufficient showcase toy %q", asset.GetShowcaseToyId())
		}
		assets[asset.GetUniqueNumber()] = asset
		toys[asset.GetShowcaseToyId()] = masterToy
	}

	arrangements := make(map[int32]*papi.ShowcaseToyArrangementInfo, len(layout.GetShowcaseToyArrangementInfos()))
	referenceCounts := make(map[int32]int32, len(assets))
	for _, arrangement := range layout.GetShowcaseToyArrangementInfos() {
		if arrangement == nil || arrangement.GetUniqueNumber() <= 0 {
			return nil, viber.InvalidArgumentf("showcase arrangement uniqueNumber must be positive")
		}
		if _, duplicate := arrangements[arrangement.GetUniqueNumber()]; duplicate {
			return nil, viber.InvalidArgumentf("showcase arrangement uniqueNumber values must be unique")
		}
		asset := assets[arrangement.GetToyAssetInfoUniqueNumber()]
		if asset == nil {
			return nil, viber.InvalidArgumentf("showcase arrangement references an unknown toy asset")
		}
		masterToy := toys[asset.GetShowcaseToyId()]
		requiredPutType := penum.ShowcaseToyPutType_ShowcaseToyPutType_Unknown
		switch arrangement.GetShowcaseFaceNumber() {
		case 0:
			requiredPutType = penum.ShowcaseToyPutType_ShowcaseToyPutType_Floor
		case 1:
			requiredPutType = penum.ShowcaseToyPutType_ShowcaseToyPutType_Wall
		default:
			return nil, viber.InvalidArgumentf("showcaseFaceNumber must be 0 or 1")
		}
		if !slices.Contains(masterToy.GetPutTypes(), requiredPutType) {
			return nil, status.Error(codes.FailedPrecondition, "showcase toy cannot be placed on the requested face")
		}
		if !finiteShowcaseCoordinate(arrangement.GetPositionX()) ||
			!finiteShowcaseCoordinate(arrangement.GetPositionY()) ||
			!finiteShowcaseCoordinate(arrangement.GetRotation()) {
			return nil, viber.InvalidArgumentf("showcase arrangement coordinates must be finite")
		}
		arrangements[arrangement.GetUniqueNumber()] = arrangement
		referenceCounts[arrangement.GetToyAssetInfoUniqueNumber()]++
	}
	for uniqueNumber, asset := range assets {
		if referenceCounts[uniqueNumber] != asset.GetCount() {
			return nil, viber.InvalidArgumentf("showcase toy asset count must match its arrangements")
		}
	}
	for _, arrangement := range arrangements {
		parent := arrangement.GetParentUniqueNumber()
		if parent != 0 {
			if parent == arrangement.GetUniqueNumber() || arrangements[parent] == nil {
				return nil, viber.InvalidArgumentf("showcase arrangement parent is invalid")
			}
			if showcaseArrangementHasCycle(arrangements, arrangement.GetUniqueNumber()) {
				return nil, viber.InvalidArgumentf("showcase arrangement parents contain a cycle")
			}
		}
	}
	return toys, nil
}

func deriveShowcaseHashtags(
	ctx context.Context,
	layout *papi.ShowcaseLayoutInfo,
	toys map[string]*pmaster.ShowcaseToy,
) ([]string, error) {
	counts := make(map[string]int64)
	for _, asset := range layout.GetShowcaseToyAssetInfos() {
		masterToy := toys[asset.GetShowcaseToyId()]
		if masterToy == nil {
			continue
		}
		for _, hashtagID := range masterToy.GetHashTagIds() {
			counts[hashtagID] += int64(asset.GetCount())
		}
	}
	if len(counts) == 0 {
		return []string{}, nil
	}
	masters, err := opmaster.FindAllShowcaseHashtags(ctx)
	if err != nil {
		return nil, err
	}
	numbers := make(map[string]int32, len(masters))
	for _, master := range masters {
		numbers[master.GetId()] = master.GetNumber()
	}
	ids := make([]string, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if counts[ids[i]] != counts[ids[j]] {
			return counts[ids[i]] > counts[ids[j]]
		}
		left, leftExists := numbers[ids[i]]
		right, rightExists := numbers[ids[j]]
		if leftExists != rightExists {
			return leftExists
		}
		if left != right {
			return left < right
		}
		return ids[i] < ids[j]
	})
	if len(ids) > showcaseMaximumHashtagCount {
		ids = ids[:showcaseMaximumHashtagCount]
	}
	return ids, nil
}

func consumeShowcaseThumbnailImageID(ctx context.Context, suid, assetID string, updated *papi.UpdatedData) error {
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return err
	}
	reserved := make(map[string]struct{}, len(user.GetNextPhotoImageIds()))
	for _, imageID := range user.GetNextPhotoImageIds() {
		reserved[imageID] = struct{}{}
	}
	if _, exists := reserved[assetID]; !exists {
		return status.Errorf(codes.FailedPrecondition, "showcase thumbnail image ID %q is not reserved for this user", assetID)
	}
	remaining := make([]string, 0, reservedPhotoImageIDCount)
	for _, imageID := range user.GetNextPhotoImageIds() {
		if imageID != assetID {
			remaining = append(remaining, imageID)
		}
	}
	for len(remaining) < reservedPhotoImageIDCount {
		candidate := uuid.NewString()
		if _, exists := reserved[candidate]; exists {
			continue
		}
		reserved[candidate] = struct{}{}
		remaining = append(remaining, candidate)
	}
	user.NextPhotoImageIds = remaining[:reservedPhotoImageIDCount]
	if err := opuser.UpsertUser(ctx, suid, user); err != nil {
		return err
	}
	updated.User = user
	return nil
}

func showcasePhotoCharacters(ctx context.Context, layout *papi.ShowcaseLayoutInfo) ([]string, error) {
	characters := make([]string, 0)
	seenToys := make(map[string]struct{})
	for _, asset := range layout.GetShowcaseToyAssetInfos() {
		if asset == nil {
			continue
		}
		if _, seen := seenToys[asset.GetShowcaseToyId()]; seen {
			continue
		}
		seenToys[asset.GetShowcaseToyId()] = struct{}{}
		toy, err := opmaster.FindShowcaseToy(ctx, asset.GetShowcaseToyId())
		if err != nil {
			return nil, err
		}
		for _, characterID := range toy.GetPhotoShootingCharacterIds() {
			if characterID != "" && !slices.Contains(characters, characterID) {
				characters = append(characters, characterID)
			}
		}
	}
	return characters, nil
}

func showcaseArrangementHasCycle(arrangements map[int32]*papi.ShowcaseToyArrangementInfo, start int32) bool {
	seen := make(map[int32]struct{})
	current := start
	for current != 0 {
		if _, duplicate := seen[current]; duplicate {
			return true
		}
		seen[current] = struct{}{}
		arrangement := arrangements[current]
		if arrangement == nil {
			return false
		}
		current = arrangement.GetParentUniqueNumber()
	}
	return false
}

func finiteShowcaseCoordinate(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

func emptyShowcaseLayout() *papi.ShowcaseLayoutInfo {
	return &papi.ShowcaseLayoutInfo{
		ShowcaseToyAssetInfos:       []*papi.ShowcaseToyAssetInfo{},
		ShowcaseToyArrangementInfos: []*papi.ShowcaseToyArrangementInfo{},
	}
}

func currentShowcaseTermInfo() *papi.ShowcaseTermInfo {
	now := time.Now().In(showcaseJST)
	return &papi.ShowcaseTermInfo{Month: int32(now.Month()), Day: int32(now.Day()), Hour: int32(now.Hour())}
}

func validShowcaseMysetNumber(number int32) bool {
	return number > 0 && number <= showcaseMysetSlotCount
}

func showcaseSlotConditionID(number int32) string {
	if number == showcaseSlotCount {
		return showcaseFifthSlotConditionID
	}
	return ""
}

func showcaseDefaultName(number int32) string {
	return "ケース" + int32String(number)
}

func showcaseDefaultMysetName(number int32) string {
	return "マイセット" + int32String(number)
}

func int32String(value int32) string {
	return strconv.FormatInt(int64(value), 10)
}
