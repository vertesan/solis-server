package umodel

import (
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// UserDreamArea is independent of monthly seasons. MaterialPhoto is a frozen
// snapshot, not an owned photo; it is retained for later quality enhancement.
type UserDreamArea struct {
	SUID              string                           `bson:"SUID"`
	BasePhotoID       string                           `bson:"basePhotoId"`
	Data              *papi.DreamUserPhoto             `bson:"data"`
	OriginalAbilities []*ptransaction.UserPhotoAbility `bson:"originalAbilities"`
	MaterialPhoto     *ptransaction.UserPhoto          `bson:"materialPhoto"`
	Quests            []*papi.DreamQuestInfo           `bson:"quests"`
}
