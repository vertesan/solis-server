package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserShowcaseMusic(ctx context.Context, suid, showcaseMusicId string) (*ptransaction.UserShowcaseMusic, error) {
	return ucommon.FindOne[ptransaction.UserShowcaseMusic](
		ctx,
		uname.UserShowcaseMusic,
		bson.M{"SUID": suid, "showcaseMusicId": showcaseMusicId},
	)
}

func FindAllUserShowcaseMusics(ctx context.Context, suid string) ([]*ptransaction.UserShowcaseMusic, error) {
	return ucommon.FindAll[ptransaction.UserShowcaseMusic](
		ctx,
		uname.UserShowcaseMusic,
		bson.M{"SUID": suid},
	)
}

func UpsertUserShowcaseMusic(ctx context.Context, suid string, music *ptransaction.UserShowcaseMusic) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserShowcaseMusic,
		bson.M{"SUID": suid, "showcaseMusicId": music.ShowcaseMusicId},
		&umodel.UserStruct[*ptransaction.UserShowcaseMusic]{SUID: suid, Data: music},
	)
}

func DeleteUserShowcaseMusic(ctx context.Context, suid, showcaseMusicId string) error {
	return ucommon.Delete(
		ctx,
		uname.UserShowcaseMusic,
		bson.M{"SUID": suid, "showcaseMusicId": showcaseMusicId},
	)
}

func DeleteAllUserShowcaseMusics(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserShowcaseMusic, bson.M{"SUID": suid})
}
