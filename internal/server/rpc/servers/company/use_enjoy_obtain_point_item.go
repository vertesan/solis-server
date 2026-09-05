package company

import (
	"context"
	"errors"
	"math"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *CompanyServer) UseEnjoyObtainPointItem(
	ctx context.Context,
	req *papi.CompanyUseObtainEnjoyPointItemRequest,
) (*papi.CompanyUseObtainEnjoyPointItemResponse, error) {
	if req.GetCharacterId() == "" || req.GetItemId() == "" || req.GetConsumptionItemAmount() <= 0 {
		return nil, viber.InvalidArgumentf("characterId, itemId, and a positive consumptionItemAmount are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyUseObtainEnjoyPointItemResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := requireOwnedCompanyCharacter(ctx, suid, req.GetCharacterId()); err != nil {
			return nil, err
		}
		if err := viber.ReconcileCompanyMissions(ctx, suid); err != nil {
			return nil, err
		}
		setting, err := opmaster.FindCompanyEnjoyObtainPointItemSetting(ctx, req.GetCharacterId(), req.GetItemId())
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, viber.InvalidArgumentf("item is unavailable for this character")
			}
			return nil, err
		}
		if setting.GetObtainPoint() <= 0 || int64(req.GetConsumptionItemAmount()) > math.MaxInt64/int64(setting.GetObtainPoint()) {
			return nil, viber.FailedPreconditionf("master enjoy point amount is invalid")
		}
		updated := &papi.UpdatedData{}
		if _, err := viber.ConsumeExchangeItemStacks(
			ctx, suid, req.GetItemId(), int64(req.GetConsumptionItemAmount()), updated,
		); err != nil {
			return nil, err
		}
		pointAmount := int64(setting.GetObtainPoint()) * int64(req.GetConsumptionItemAmount())
		if _, _, err := viber.AddCompanyEnjoyPoints(ctx, suid, req.GetCharacterId(), pointAmount, updated); err != nil {
			return nil, err
		}
		characterMissions, err := advanceCompanyMissions(
			ctx,
			suid,
			penum.MissionType_MissionType_CompanyCharacterUseObtainEnjoyPointItemCount,
			int64(req.GetConsumptionItemAmount()),
			req.GetCharacterId(),
		)
		if err != nil {
			return nil, err
		}
		totalMissions, err := advanceCompanyMissions(
			ctx,
			suid,
			penum.MissionType_MissionType_CompanyUseObtainEnjoyPointItemTotalCount,
			int64(req.GetConsumptionItemAmount()),
		)
		if err != nil {
			return nil, err
		}
		for _, mission := range append(characterMissions, totalMissions...) {
			putUpdatedCompanyMission(updated, mission)
		}
		totalCount, err := opuser.FindUserTotalCount(ctx, suid)
		if err != nil {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return nil, err
			}
			totalCount = &ptransaction.UserTotalCount{}
			if err := opuser.UpsertUserTotalCount(ctx, suid, totalCount); err != nil {
				return nil, err
			}
		}
		updated.TotalCount = totalCount
		response = &papi.CompanyUseObtainEnjoyPointItemResponse{
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		if errors.Is(err, viber.ErrExchangeInsufficientResource) {
			return nil, status.Error(codes.ResourceExhausted, "insufficient item amount")
		}
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

// Company mission progress is first reported as Clear. A later progress event
// changes that state to NotReceived, which matches the two-step transition in
// the captured gift-use sequence.
func advanceCompanyMissions(
	ctx context.Context,
	suid string,
	missionType penum.MissionType,
	amount int64,
	targetIDs ...string,
) ([]*ptransaction.UserMission, error) {
	existing, err := opuser.FindAllUserMissions(ctx, suid)
	if err != nil {
		return nil, err
	}
	type missionSnapshot struct {
		status    penum.MissionStatusType
		progress  int64
		threshold int64
	}
	before := make(map[string]missionSnapshot, len(existing))
	for _, mission := range existing {
		before[mission.GetMissionId()] = missionSnapshot{
			status:    mission.GetStatusType(),
			progress:  mission.GetProgress(),
			threshold: mission.GetCurrentThreshold(),
		}
	}

	changed, err := viber.AdvanceUserMissions(ctx, suid, missionType, amount, targetIDs...)
	if err != nil {
		return nil, err
	}
	for _, mission := range changed {
		previous, ok := before[mission.GetMissionId()]
		if !ok || previous.status != penum.MissionStatusType_MissionStatusType_Progress ||
			previous.progress >= previous.threshold || mission.GetProgress() < previous.threshold {
			continue
		}
		mission.StatusType = penum.MissionStatusType_MissionStatusType_Clear
		if err := opuser.UpsertUserMission(ctx, suid, mission); err != nil {
			return nil, err
		}
	}
	return changed, nil
}
