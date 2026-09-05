package auth

import (
	"github.com/vertesan/solis-server/internal/proto/papi"
)

type AuthServer struct {
	papi.UnimplementedAuthServer
}
