package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserLeagueDeckPositions(ctx context.Context, suid string) ([]*ptransaction.UserLeagueDeckPosition, error) {
	return ucommon.FindAll[ptransaction.UserLeagueDeckPosition](
		ctx,
		uname.UserLeagueDeckPosition,
		bson.M{"SUID": suid},
	)
}

func UpsertUserLeagueDeckPosition(ctx context.Context, suid string, position *ptransaction.UserLeagueDeckPosition) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserLeagueDeckPosition,
		bson.M{
			"SUID":           suid,
			"leagueSeasonId": position.LeagueSeasonId,
			"deckType":       position.DeckType,
			"position":       position.Position,
		},
		&umodel.UserStruct[*ptransaction.UserLeagueDeckPosition]{SUID: suid, Data: position},
	)
}

func UpsertManyUserLeagueDeckPositions(ctx context.Context, suid string, positions []*ptransaction.UserLeagueDeckPosition) error {
	return ucommon.ForEachAtomically(ctx, positions, func(ctx context.Context, position *ptransaction.UserLeagueDeckPosition) error {
		return UpsertUserLeagueDeckPosition(ctx, suid, position)
	})
}
