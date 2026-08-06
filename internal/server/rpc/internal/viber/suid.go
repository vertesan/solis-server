package viber

import (
	"context"

	"github.com/vertesan/solis-server/internal/server/rpc/internal/interceptor"
	"github.com/vertesan/solis-server/internal/utils/rich"
)

func GetSuid(ctx context.Context) string {
	val := ctx.Value(interceptor.AuthCtxKey)
	suid, ok := val.(string)
	if !ok {
		rich.Warn("failed to convert AuthCtxKey to string")
		return ""
	}
	return suid
}
