package umodel

import "github.com/vertesan/solis-server/internal/proto/papi"

type UserMarketState struct {
	SUID                  string             `bson:"SUID"`
	MarketItems           []*papi.MarketItem `bson:"marketItems"`
	InventoryBoundaryTime int64              `bson:"inventoryBoundaryTime"`
	FreeResetBoundaryTime int64              `bson:"freeResetBoundaryTime"`
	PaidResetDayTime      int64              `bson:"paidResetDayTime"`
	PaidResetCount        int32              `bson:"paidResetCount"`
}

type UserExcursionState struct {
	SUID             string   `bson:"SUID"`
	OfferedCharacter string   `bson:"offeredCharacter"`
	OfferedPlaceIds  []string `bson:"offeredPlaceIds"`
	ActiveCharacter  string   `bson:"activeCharacter"`
	ActivePlaceId    string   `bson:"activePlaceId"`
	StartedTime      int64    `bson:"startedTime"`
}
