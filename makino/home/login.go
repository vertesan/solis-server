package home

import (
	"context"
	"time"
	"vertesan/solis-server/db/store/opuser"
	"vertesan/solis-server/proto/papi"
)

var jstZone = time.FixedZone("JST", 9*60*60)

func Login(ctx context.Context, suid string) (*papi.HomeLoginResponse, error) {
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	prevLogin := time.UnixMilli(user.LastLoginTime).In(jstZone)
	nowJst := now.In(jstZone)
	isFirstOfDay := prevLogin.Year() != nowJst.Year() ||
		prevLogin.YearDay() != nowJst.YearDay()

	user.LastLoginTime = now.UnixMilli()
	if err := opuser.UpsertUser(ctx, suid, user); err != nil {
		return nil, err
	}

	updatedData := &papi.UpdatedData{}
	if isFirstOfDay {
		totalCount, err := opuser.FindUserTotalCount(ctx, suid)
		if err != nil {
			return nil, err
		}
		totalCount.LoginCount++
		if err := opuser.UpsertUserTotalCount(ctx, suid, totalCount); err != nil {
			return nil, err
		}
		updatedData.User = user
		updatedData.TotalCount = totalCount
	}

	return &papi.HomeLoginResponse{
		CommonResponse: &papi.CommonResponse{
			UpdatedData: updatedData,
			DeletedData: &papi.DeletedData{},
		},
	}, nil
}
