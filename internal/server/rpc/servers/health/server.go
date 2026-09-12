package health

import "github.com/vertesan/solis-server/internal/proto/papi"

type HealthServer struct {
	papi.UnimplementedHealthServer
}
