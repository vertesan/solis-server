package servers

import (
	"context"
	"errors"
	"sort"
	"time"

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
)

type LoginBonusServer struct {
	papi.UnimplementedLoginBonusServer
}

func (s *LoginBonusServer) List(ctx context.Context, _ *papi.Empty) (*papi.LoginBonusListResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.LoginBonusListResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
		masters, err := opmaster.FindAllLoginBonuses(ctx)
		if err != nil {
			return nil, err
		}
		sort.SliceStable(masters, func(i, j int) bool {
			if masters[i].GetPriority() == masters[j].GetPriority() {
				return masters[i].GetId() < masters[j].GetId()
			}
			return masters[i].GetPriority() > masters[j].GetPriority()
		})
		states, err := opuser.FindAllUserLoginBonuses(ctx, suid)
		if err != nil {
			return nil, err
		}
		stateByID := make(map[string]*ptransaction.UserLoginBonus, len(states))
		for _, state := range states {
			stateByID[state.GetLoginBonusId()] = state
		}
		texts, err := eligibleLoginBonusTexts(ctx, suid)
		if err != nil {
			return nil, err
		}
		evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now)
		infos := make([]*papi.LoginBonusInfo, 0, len(masters))
		for _, master := range masters {
			if master.GetId() == "" || len(master.GetRewardIds()) == 0 {
				continue
			}
			open, err := evaluator.Satisfied(master.GetOpenConditionId())
			if err != nil {
				return nil, err
			}
			if !open {
				continue
			}
			state := stateByID[master.GetId()]
			changed := false
			if state == nil {
				state = &ptransaction.UserLoginBonus{LoginBonusId: master.GetId(), LastLoginTime: now, LoginCount: 1}
				changed = true
			} else if state.GetLastLoginTime() <= 0 || !viber.SameJSTDay(state.GetLastLoginTime(), now) {
				state.LastLoginTime = now
				if master.GetType() == penum.LoginBonusType_LoginBonusType_Normal || state.GetLoginCount() < int32(len(master.GetRewardIds())) {
					state.LoginCount++
				}
				changed = true
			}
			if state.GetLoginCount() <= 0 {
				state.LoginCount = 1
				changed = true
			}
			if master.GetType() == penum.LoginBonusType_LoginBonusType_Normal && len(texts) > 0 && (state.GetLoginBonusTextId() == "" || changed) {
				state.LoginBonusTextId = texts[int(state.GetLoginCount()-1)%len(texts)].GetId()
			}
			if master.GetType() != penum.LoginBonusType_LoginBonusType_Normal && state.GetReceiveCount() >= int32(len(master.GetRewardIds())) {
				if changed {
					if err := opuser.UpsertUserLoginBonus(ctx, suid, state); err != nil {
						return nil, err
					}
				}
				continue
			}
			if changed {
				if err := opuser.UpsertUserLoginBonus(ctx, suid, state); err != nil {
					return nil, err
				}
			}
			infos = append(infos, &papi.LoginBonusInfo{UserLoginBonus: state})
		}
		response = &papi.LoginBonusListResponse{Infos: infos, CommonResponse: viber.NewCommonResponse(nil)}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

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

func eligibleLoginBonusTexts(ctx context.Context, suid string) ([]*pmaster.LoginBonusText, error) {
	texts, err := opmaster.FindAllLoginBonusTexts(ctx)
	if err != nil {
		return nil, err
	}
	costumes, err := opuser.FindAllUserCostumes(ctx, suid)
	if err != nil {
		return nil, err
	}
	owned := make(map[string]struct{}, len(costumes))
	for _, costume := range costumes {
		owned[costume.GetCostumeId()] = struct{}{}
	}
	eligible := make([]*pmaster.LoginBonusText, 0, len(texts))
	for _, text := range texts {
		if _, ok := owned[text.GetCostumeId()]; ok {
			eligible = append(eligible, text)
		}
	}
	return eligible, nil
}
