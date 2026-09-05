package user

import (
	"github.com/vertesan/solis-server/internal/proto/papi"
)

type UserServer struct {
	papi.UnimplementedUserServer
}
