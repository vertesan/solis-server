package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserGachaInfo(ctx context.Context, suid string, id string) (*papi.GachaInfo, error) {
	return ucommon.FindOne[papi.GachaInfo](
		ctx,
		uname.UserGachaInfo,
		bson.M{"SUID": suid, "id": id},
	)
}

func FindAllUserGachaInfos(ctx context.Context, suid string) ([]*papi.GachaInfo, error) {
	return ucommon.FindAll[papi.GachaInfo](
		ctx,
		uname.UserGachaInfo,
		bson.M{"SUID": suid},
	)
}

func UpsertUserGachaInfo(ctx context.Context, suid string, info *papi.GachaInfo) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserGachaInfo,
		bson.M{"SUID": suid, "id": info.Id},
		&umodel.UserStruct[*papi.GachaInfo]{
			SUID: suid,
			Data: info,
		},
	)
}

func UpsertManyUserGachaInfos(ctx context.Context, suid string, infos []*papi.GachaInfo) error {
	for _, info := range infos {
		if err := UpsertUserGachaInfo(ctx, suid, info); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserGachaInfos(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserGachaInfo,
		bson.M{"SUID": suid},
	)
}

func FindAllUserGachaButtons(ctx context.Context, suid string) ([]*ptransaction.UserGachaButton, error) {
	infos, err := FindAllUserGachaInfos(ctx, suid)
	if err != nil {
		return nil, err
	}
	buttons := make([]*ptransaction.UserGachaButton, 0)
	for _, info := range infos {
		for _, btn := range info.Buttons {
			buttons = append(buttons, &ptransaction.UserGachaButton{
				GachaButtonId: btn.Id,
				DrawTime:      btn.DrawTime,
				TodayCount:    btn.TodayCount,
				TotalCount:    btn.TotalCount,
			})
		}
	}
	return buttons, nil
}
