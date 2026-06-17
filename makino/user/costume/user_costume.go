package costume

import (
	"context"
	"time"
	"vertesan/solis-server/db/store/opmaster"
	"vertesan/solis-server/proto/pmaster"
	"vertesan/solis-server/proto/ptransaction"
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
