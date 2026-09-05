package showcase

import (
	"context"
	"errors"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func currentShowcaseTermInfo() *papi.ShowcaseTermInfo {
	now := time.Now().In(showcaseJST)
	return &papi.ShowcaseTermInfo{Month: int32(now.Month()), Day: int32(now.Day()), Hour: int32(now.Hour())}
}

func validShowcaseMysetNumber(number int32) bool {
	return number > 0 && number <= showcaseMysetSlotCount
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
