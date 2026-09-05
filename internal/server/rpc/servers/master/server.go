package master

import (
	"github.com/vertesan/solis-server/internal/proto/papi"
)

type MasterServer struct {
	papi.UnimplementedMasterServer
}
