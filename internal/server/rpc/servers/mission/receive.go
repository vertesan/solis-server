package mission

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (s *MissionServer) Receive(ctx context.Context, req *papi.MissionReceiveRequest) (*papi.MissionReceiveResponse, error) {
	if len(req.GetMissionIds()) == 0 {
		return nil, viber.InvalidArgumentf("missionIds is required")
	}
	seen := map[string]bool{}
	for _, id := range req.GetMissionIds() {
		if id == "" || seen[id] {
			return nil, viber.InvalidArgumentf("missionIds must be non-empty and unique")
		}
		seen[id] = true
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.MissionReceiveResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
		updated := &papi.UpdatedData{}
		catalog, err := viber.LoadActiveMissionCatalog(ctx, suid, now)
		if err != nil {
			return nil, err
		}
		if err := catalog.Reconcile(ctx, suid, now, updated); err != nil {
			return nil, err
		}
		results := []*ptransaction.RewardResult{}
		for _, id := range req.GetMissionIds() {
			master := catalog.Missions[id]
			if master == nil {
				return nil, viber.FailedPreconditionf("mission %q is not active", id)
			}
			row, err := opuser.FindUserMission(ctx, suid, id)
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, viber.FailedPreconditionf("mission %q is not active", id)
			}
			if err != nil {
				return nil, err
			}
			if (row.GetStatusType() != penum.MissionStatusType_MissionStatusType_Clear && row.GetStatusType() != penum.MissionStatusType_MissionStatusType_NotReceived) || row.GetProgress() < row.GetCurrentThreshold() {
				return nil, viber.FailedPreconditionf("mission %q reward is not receivable", id)
			}
			progress, err := missionThreshold(master, row.GetCurrentThreshold())
			if err != nil {
				return nil, err
			}
			resources, err := viber.ResolveRewardResources(ctx, progress.GetRewardSetId(), 1)
			if err != nil {
				return nil, err
			}
			for _, resource := range resources {
				granted, err := viber.GrantGachaReward(ctx, suid, resource, nil, updated)
				if err != nil {
					return nil, err
				}
				results = append(results, granted...)
			}
			// Rewards such as gold can themselves advance this mission. Keep
			// that latest progress when acknowledging the tier in the same transaction.
			row, err = opuser.FindUserMission(ctx, suid, id)
			if err != nil {
				return nil, err
			}
			// One requested ID claims exactly one tier, even if gameplay has
			// already reached several thresholds (as in the official captures).
			next := int64(0)
			for _, tier := range master.GetProgresses() {
				if tier.GetThreshold() > row.GetCurrentThreshold() && (next == 0 || tier.GetThreshold() < next) {
					next = tier.GetThreshold()
				}
			}
			row.StatusType = penum.MissionStatusType_MissionStatusType_Received
			if next > 0 {
				row.CurrentThreshold = next
				row.StatusType = penum.MissionStatusType_MissionStatusType_Progress
				if row.GetProgress() >= next {
					row.StatusType = penum.MissionStatusType_MissionStatusType_Clear
				}
			}
			// Receipt does not change lastUpdateTime: it records gameplay, and
			// is also the reset/login-day boundary for recurring missions.
			if err := opuser.UpsertUserMission(ctx, suid, row); err != nil {
				return nil, err
			}
			viber.PutUpdatedMission(updated, row)
			if next > 0 && row.GetProgress() >= next {
				parents, err := viber.AdvanceUserMissions(ctx, suid, penum.MissionType_MissionType_MissionClearCount, 1, id)
				if err != nil {
					return nil, err
				}
				for _, parent := range parents {
					viber.PutUpdatedMission(updated, parent)
				}
			}
		}
		results, err = viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}
		response = &papi.MissionReceiveResponse{Results: results, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}

func missionThreshold(master *pmaster.Mission, threshold int64) (*pmaster.MissionProgress, error) {
	var found *pmaster.MissionProgress
	for _, progress := range master.GetProgresses() {
		if progress.GetThreshold() != threshold {
			continue
		}
		if found != nil {
			return nil, viber.FailedPreconditionf("mission %q has duplicate thresholds", master.GetId())
		}
		found = progress
	}
	if found == nil || found.GetRewardSetId() == "" {
		return nil, viber.FailedPreconditionf("mission %q threshold is invalid", master.GetId())
	}
	return found, nil
}
