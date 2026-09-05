package viber

import "github.com/vertesan/solis-server/internal/proto/ptransaction"

// ReservedAccessoryAmount counts copies needed by the most demanding deck.
// The same inventory can be reused in different saved formations.
func ReservedAccessoryAmount(
	id string,
	positions []*ptransaction.UserDeckPosition,
	leaguePositions []*ptransaction.UserLeagueDeckPosition,
) int64 {
	var reserved int64
	deckCounts := make(map[int32]int64)
	for _, position := range positions {
		if position.Part1AccessoryId == id {
			deckCounts[position.Number]++
		}
		if position.Part2AccessoryId == id {
			deckCounts[position.Number]++
		}
		reserved = max(reserved, deckCounts[position.Number])
	}
	type leagueDeckKey struct {
		season string
		typeID int32
	}
	leagueCounts := make(map[leagueDeckKey]int64)
	for _, position := range leaguePositions {
		key := leagueDeckKey{position.LeagueSeasonId, int32(position.DeckType)}
		if position.Part1AccessoryId == id {
			leagueCounts[key]++
		}
		if position.Part2AccessoryId == id {
			leagueCounts[key]++
		}
		reserved = max(reserved, leagueCounts[key])
	}
	return reserved
}
