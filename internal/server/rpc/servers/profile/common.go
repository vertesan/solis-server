package profile

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func updateProfile(ctx context.Context, mutate func(context.Context, string, *ptransaction.UserProfile) error) (*papi.CommonResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.CommonResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		profile, err := opuser.FindUserProfile(ctx, suid)
		if err != nil {
			return nil, err
		}
		if err := mutate(ctx, suid, profile); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserProfile(ctx, suid, profile); err != nil {
			return nil, err
		}
		missions, err := viber.ReachUserMissions(ctx, suid, penum.MissionType_MissionType_ProfileSetting, 1)
		if err != nil {
			return nil, err
		}
		response = viber.NewCommonResponse(&papi.UpdatedData{Profile: profile, Missions: missions})
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func updateProfileUser(ctx context.Context, mutate func(context.Context, string, *ptransaction.User) error) (*papi.CommonResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.CommonResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		if err := mutate(ctx, suid, user); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUser(ctx, suid, user); err != nil {
			return nil, err
		}
		missions, err := viber.ReachUserMissions(ctx, suid, penum.MissionType_MissionType_ProfileSetting, 1)
		if err != nil {
			return nil, err
		}
		response = viber.NewCommonResponse(&papi.UpdatedData{User: user, Missions: missions})
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func profileHierarchyGrade(ctx context.Context, suid string) (string, error) {
	hierarchy, err := opuser.FindUserHierarchy(ctx, suid)
	if err != nil {
		return "", err
	}
	point, err := viber.HierarchyTotalPoint(hierarchy)
	if err != nil {
		return "", err
	}
	grades, err := opmaster.FindAllHierarchyDetailGrades(ctx)
	if err != nil {
		return "", err
	}
	grade, err := viber.ResolveHierarchyDetailGrade(grades, point, viber.HierarchyPrivateServerRank)
	if err != nil {
		return "", err
	}
	return grade.GetId(), nil
}
