package servers

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

type DivisionServer struct {
	papi.UnimplementedDivisionServer
}

// List returns the live server-instance roster. Division master data defines
// ranks/names, while membership and aggregate levels are mutable server state.
func (s *DivisionServer) List(ctx context.Context, _ *papi.Empty) (*papi.DivisionListResponse, error) {
	divisions, err := opsolis.FindAllDivisions(ctx)
	if err != nil {
		return nil, err
	}
	return &papi.DivisionListResponse{
		Divisions:      divisions,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
