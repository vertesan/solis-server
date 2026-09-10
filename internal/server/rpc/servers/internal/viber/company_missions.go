package viber

import (
	"context"
	"sort"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// ReconcileCompanyMissions creates the visible Company mission rows which are
// absent from a user's data and projects recorded home talk achievements.
// Reconciliation never reduces existing progress or reopens claimed missions.
func ReconcileCompanyMissions(ctx context.Context, suid string) error {
	masters, err := opmaster.FindAllMissions(ctx)
	if err != nil {
		return err
	}
	userMissions, err := opuser.FindAllUserMissions(ctx, suid)
	if err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(userMissions))
	for _, mission := range userMissions {
		existing[mission.GetMissionId()] = struct{}{}
	}
	characters, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		return err
	}
	ownedCharacters := make(map[string]struct{}, len(characters))
	for _, character := range characters {
		ownedCharacters[character.GetCharacterId()] = struct{}{}
	}

	candidates := make([]*pmaster.Mission, 0)
	for _, master := range masters {
		isTrust := master.GetCompanyTrustMissionCategoryType() != penum.CompanyTrustMissionCategoryType_CompanyTrustMissionCategoryType_Unknown
		characterID := master.GetCompanyEnjoyCharacterId()
		if master.GetId() == "" || !isTrust && characterID == "" {
			continue
		}
		if characterID != "" {
			if _, owned := ownedCharacters[characterID]; !owned {
				continue
			}
		}
		if _, exists := existing[master.GetId()]; exists {
			continue
		}
		if _, ok := firstCompanyMissionThreshold(master.GetProgresses()); ok {
			candidates = append(candidates, master)
		}
	}
	if len(candidates) == 0 {
		_, err := ReachCharacterHomeTalkMissions(ctx, suid, "")
		return err
	}
	conditions, err := opmaster.FindAllConditions(ctx)
	if err != nil {
		return err
	}
	evaluator := NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
	evaluator.PrimeConditions(conditions)

	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].GetId() < candidates[j].GetId() })
	created := make([]*ptransaction.UserMission, 0)
	for _, master := range candidates {
		visible, err := evaluator.Satisfied(master.GetViewConditionId())
		if err != nil {
			return err
		}
		if !visible {
			continue
		}
		threshold, ok := firstCompanyMissionThreshold(master.GetProgresses())
		if !ok {
			continue
		}
		created = append(created, &ptransaction.UserMission{
			MissionId:        master.GetId(),
			CurrentThreshold: threshold,
			StatusType:       penum.MissionStatusType_MissionStatusType_Progress,
		})
	}
	if err := opuser.UpsertManyUserMissions(ctx, suid, created); err != nil {
		return err
	}
	_, err = ReachCharacterHomeTalkMissions(ctx, suid, "")
	return err
}

func firstCompanyMissionThreshold(progresses []*pmaster.MissionProgress) (int64, bool) {
	var first int64
	for _, progress := range progresses {
		threshold := progress.GetThreshold()
		if threshold <= 0 {
			continue
		}
		if first == 0 || threshold < first {
			first = threshold
		}
	}
	return first, first > 0
}
