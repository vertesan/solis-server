package viber

import (
	"context"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/server/rpc/internal/interceptor"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
)

func GetSuid(ctx context.Context) *uuid.UUID {
	val := ctx.Value(interceptor.AuthTokenKey{})
	suid, ok := val.(*uuid.UUID)
	if !ok {
		mcontext.GetLogger(ctx).Warn("failed to retrieve x-auth-token from context by AuthTokenKey")
		u := uuid.MustParse("00000000-0000-0000-0000-000000000000")
		return &u
	}
	return suid
}
