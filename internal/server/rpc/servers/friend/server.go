package friend

import "github.com/vertesan/solis-server/internal/proto/papi"

// FriendServer acknowledges social requests without creating multiplayer state.
type FriendServer struct {
	papi.UnimplementedFriendServer
}
