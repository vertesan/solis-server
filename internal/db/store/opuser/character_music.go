package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func UpsertUserCharacterMusic(ctx context.Context, suid string, row *ptransaction.UserCharacterMusic) error {
	return ucommon.UpsertOne(ctx, uname.UserCharacterMusic,
		bson.M{"SUID": suid, "musicId": row.GetMusicId(), "characterId": row.GetCharacterId()},
		&umodel.UserStruct[*ptransaction.UserCharacterMusic]{SUID: suid, Data: row})
}

func FindUserMusicMasteries(ctx context.Context, suid, musicID string) ([]*ptransaction.UserCharacterMusic, error) {
	return ucommon.FindAll[ptransaction.UserCharacterMusic](ctx, uname.UserCharacterMusic, bson.M{"SUID": suid, "musicId": musicID})
}

func FindAllUserCharacterMusics(ctx context.Context, suid string) ([]*ptransaction.UserCharacterMusic, error) {
	return ucommon.FindAll[ptransaction.UserCharacterMusic](
		ctx,
		uname.UserCharacterMusic,
		bson.M{"SUID": suid},
	)
}
