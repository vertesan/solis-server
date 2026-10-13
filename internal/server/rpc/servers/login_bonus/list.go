package loginbonus

import (
	"context"
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
)

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
			if master.GetId() == "" || len(master.GetRewardIds()) == 0 || !loginBonusEnabled(master) {
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
			infos = append(infos, &papi.LoginBonusInfo{UserLoginBonus: state, NoticeInfo: viber.NewDummyNoticeInfo()})
		}
		response = &papi.LoginBonusListResponse{Infos: infos, CommonResponse: viber.NewCommonResponse(nil)}
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
