package quest

import (
	"context"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
)

func (s *QuestServer) OutputLiveResultDetailLog(ctx context.Context, req *papi.OutputLiveResultDetailLogRequest) (*papi.OutputLiveResultDetailLogResponse, error) {
	// This RPC is telemetry only. Client-supplied scores must never clear a
	// quest, alter rankings or grant rewards without a validated live result.
	mcontext.GetLogger(ctx).Debugw("quest live result detail viewed",
		"live_result_detail_log_id", req.GetLiveResultDetailLogId(), "cleared", req.GetCleared(), "score", req.GetScore())
	return &papi.OutputLiveResultDetailLogResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}
