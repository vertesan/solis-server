package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserFanEventRun(ctx context.Context, suid string) (*papi.FanEventProgress, error) {
	return ucommon.FindOne[papi.FanEventProgress](ctx, uname.UserFanEventRun, bson.M{"SUID": suid})
}

func UpsertUserFanEventRun(ctx context.Context, suid string, progress *papi.FanEventProgress) error {
	return ucommon.UpsertOne(ctx, uname.UserFanEventRun, bson.M{"SUID": suid}, &umodel.UserStruct[*papi.FanEventProgress]{
		SUID: suid,
		Data: progress,
	})
}

func DeleteUserFanEventRun(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserFanEventRun, bson.M{"SUID": suid})
}

func FindUserPromotionRun(ctx context.Context, suid string) (*papi.PromotionProgress, error) {
	return ucommon.FindOne[papi.PromotionProgress](ctx, uname.UserPromotionRun, bson.M{"SUID": suid})
}

func UpsertUserPromotionRun(ctx context.Context, suid string, progress *papi.PromotionProgress) error {
	return ucommon.UpsertOne(ctx, uname.UserPromotionRun, bson.M{"SUID": suid}, &umodel.UserStruct[*papi.PromotionProgress]{
		SUID: suid,
		Data: progress,
	})
}

func FindUserPromotionRunState(ctx context.Context, suid string) (*umodel.UserPromotionRunState, error) {
	return ucommon.FindOne[umodel.UserPromotionRunState](ctx, uname.UserPromotionRun, bson.M{"SUID": suid})
}

func UpsertUserPromotionRunState(ctx context.Context, state *umodel.UserPromotionRunState) error {
	return ucommon.UpsertOne(ctx, uname.UserPromotionRun, bson.M{"SUID": state.SUID}, state)
}

func DeleteUserPromotionRun(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserPromotionRun, bson.M{"SUID": suid})
}

func FindUserRefreshRun(ctx context.Context, suid string) (*papi.RefreshProgress, error) {
	return ucommon.FindOne[papi.RefreshProgress](ctx, uname.UserRefreshRun, bson.M{"SUID": suid})
}

func UpsertUserRefreshRun(ctx context.Context, suid string, progress *papi.RefreshProgress) error {
	return ucommon.UpsertOne(ctx, uname.UserRefreshRun, bson.M{"SUID": suid}, &umodel.UserStruct[*papi.RefreshProgress]{
		SUID: suid,
		Data: progress,
	})
}

func DeleteUserRefreshRun(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserRefreshRun, bson.M{"SUID": suid})
}
