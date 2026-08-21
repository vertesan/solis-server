package umodel

import (
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
)

type UserGachaHistory struct {
	SUID      string               `bson:"SUID"`
	GachaType penum.GachaType      `bson:"gachaType"`
	Histories []*papi.GachaHistory `bson:"histories"`
}
