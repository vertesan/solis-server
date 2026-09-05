package message

import (
	"github.com/vertesan/solis-server/internal/proto/papi"
)

type MessageServer struct {
	papi.UnimplementedMessageServer
}
