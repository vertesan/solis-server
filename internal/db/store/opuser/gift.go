package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserGifts(ctx context.Context, suid string) ([]*ptransaction.UserGift, error) {
	return ucommon.FindAll[ptransaction.UserGift](ctx, uname.UserGift, bson.M{"SUID": suid})
}

func FindUserGift(ctx context.Context, suid, giftID string) (*ptransaction.UserGift, error) {
	return ucommon.FindOne[ptransaction.UserGift](ctx, uname.UserGift, bson.M{"SUID": suid, "giftId": giftID})
}

func UpsertUserGift(ctx context.Context, suid string, gift *ptransaction.UserGift) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserGift,
		bson.M{"SUID": suid, "giftId": gift.GetGiftId()},
		&umodel.UserStruct[*ptransaction.UserGift]{SUID: suid, Data: gift},
	)
}

func DeleteUserGift(ctx context.Context, suid, giftID string) error {
	return ucommon.Delete(ctx, uname.UserGift, bson.M{"SUID": suid, "giftId": giftID})
}

func FindAllUserGiftHistories(ctx context.Context, suid string) ([]*ptransaction.UserGiftHistory, error) {
	return ucommon.FindAll[ptransaction.UserGiftHistory](ctx, uname.UserGiftHistory, bson.M{"SUID": suid})
}

func UpsertUserGiftHistory(ctx context.Context, suid string, history *ptransaction.UserGiftHistory) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserGiftHistory,
		bson.M{"SUID": suid, "giftId": history.GetGiftId()},
		&umodel.UserStruct[*ptransaction.UserGiftHistory]{SUID: suid, Data: history},
	)
}
