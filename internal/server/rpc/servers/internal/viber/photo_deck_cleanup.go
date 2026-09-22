package viber

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"slices"
)

// Selling an equipped photo is allowed, but its references must be removed in
// the same transaction so every saved formation remains usable.
func DetachDeletedPhotosFromDecks(ctx context.Context, suid string, ids []string, updated *papi.UpdatedData) error {
	deleted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		deleted[id] = struct{}{}
	}
	remove := func(id string) bool {
		_, exists := deleted[id]
		return exists
	}
	positions, err := opuser.FindAllUserDeckPositions(ctx, suid)
	if err != nil {
		return err
	}
	for _, position := range positions {
		count := len(position.PhotoIds)
		position.PhotoIds = slices.DeleteFunc(position.PhotoIds, remove)
		if len(position.PhotoIds) != count {
			updated.DeckPositions = append(updated.DeckPositions, position)
		}
	}
	if err := opuser.UpsertManyUserDeckPositions(ctx, suid, updated.DeckPositions); err != nil {
		return err
	}
	leaguePositions, err := opuser.FindAllUserLeagueDeckPositions(ctx, suid)
	if err != nil {
		return err
	}
	for _, position := range leaguePositions {
		count := len(position.PhotoIds)
		position.PhotoIds = slices.DeleteFunc(position.PhotoIds, remove)
		if len(position.PhotoIds) != count {
			updated.LeagueDeckPositions = append(updated.LeagueDeckPositions, position)
		}
	}
	return opuser.UpsertManyUserLeagueDeckPositions(ctx, suid, updated.LeagueDeckPositions)
}
