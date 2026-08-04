package costume

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"time"
)

func SpawnUserCostumeById(costumeId string) (*ptransaction.UserCostume, error) {
	costume, err := opmaster.FindCostume(context.Background(), costumeId)
	if err != nil {
		return nil, err
	}
	return SpawnUserCostume(costume), nil
}

func SpawnUserCostume(costume *pmaster.Costume) *ptransaction.UserCostume {
	userCostume := &ptransaction.UserCostume{
		CostumeId:    costume.Id,
		Checked:      false,
		ObtainedTime: time.Now().UnixMilli(),
	}
	return userCostume
}
