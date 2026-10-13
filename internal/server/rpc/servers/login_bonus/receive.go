package loginbonus

import (
	"context"
	"errors"
	"time"

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

func (s *LoginBonusServer) Receive(ctx context.Context, req *papi.LoginBonusReceiveRequest) (*papi.LoginBonusReceiveResponse, error) {
	if req.GetLoginBonusId() == "" {
		return nil, status.Error(codes.InvalidArgument, "loginBonusId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.LoginBonusReceiveResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		master, err := opmaster.FindLoginBonus(ctx, req.GetLoginBonusId())
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.NotFound, "login bonus not found")
			}
			return nil, err
		}
		if !loginBonusEnabled(master) {
			return nil, status.Error(codes.PermissionDenied, "login bonus is not available")
		}
		evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
		open, err := evaluator.Satisfied(master.GetOpenConditionId())
		if err != nil {
			return nil, err
		}
		receivable, err := evaluator.Satisfied(master.GetReceiveConditionId())
		if err != nil {
			return nil, err
		}
		if !open || !receivable {
			return nil, status.Error(codes.PermissionDenied, "login bonus is not available")
		}
		state, err := opuser.FindUserLoginBonus(ctx, suid, master.GetId())
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.FailedPrecondition, "list login bonuses before receiving")
			}
			return nil, err
		}
		if state.GetReceiveCount() >= state.GetLoginCount() {
			return nil, status.Error(codes.FailedPrecondition, "today's login bonus was already received")
		}
		if len(master.GetRewardIds()) == 0 {
			return nil, status.Error(codes.FailedPrecondition, "login bonus has no rewards")
		}
		rewardIndex := int(state.GetReceiveCount())
		if master.GetType() == penum.LoginBonusType_LoginBonusType_Normal {
			rewardIndex %= len(master.GetRewardIds())
		} else if rewardIndex >= len(master.GetRewardIds()) {
			return nil, status.Error(codes.FailedPrecondition, "login bonus is complete")
		}
		resources, err := viber.ParseAutoReward(master.GetRewardIds()[rewardIndex])
		if err != nil {
			return nil, err
		}
		if resources == nil {
			reward, err := opmaster.FindReward(ctx, master.GetRewardIds()[rewardIndex])
			if err != nil {
				return nil, err
			}
			resources = reward.GetResources()
		}
		updated := &papi.UpdatedData{}
		results := make([]*ptransaction.RewardResult, 0, len(resources))
		for _, resource := range resources {
			if resource.GetAmount() <= 0 {
				return nil, status.Error(codes.FailedPrecondition, "login bonus reward amount must be positive")
			}
			granted, err := viber.GrantExchangeRewards(ctx, suid, resource, updated)
			if err != nil {
				return nil, err
			}
			results = append(results, granted...)
		}
		state.ReceiveCount++
		if err := opuser.UpsertUserLoginBonus(ctx, suid, state); err != nil {
			return nil, err
		}
		if master.GetType() == penum.LoginBonusType_LoginBonusType_Normal {
			notification := &ptransaction.UserNotification{NotificationType: penum.NotificationType_NotificationType_LoginBonus}
			if err := opuser.UpsertUserNotification(ctx, suid, notification); err != nil {
				return nil, err
			}
			updated.Notifications = append(updated.Notifications, notification)
		}
		response = &papi.LoginBonusReceiveResponse{
			UserLoginBonus: state,
			Rewards:        results,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}
