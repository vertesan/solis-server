package staff

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const staffLevelLimitBreakItemID = "item-staff-limit-break"

func validateStaffParameter(ctx context.Context, parameterType penum.ParameterType) error {
	if _, err := opmaster.FindStaffTraining(ctx, parameterType); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return viber.InvalidArgumentf("unknown staff parameterType %d", parameterType)
		}
		return viber.GameplayError(err)
	}
	return nil
}

func staffMutationError(err error, insufficientMessage string) error {
	if errors.Is(err, viber.ErrExchangeInsufficientResource) {
		return status.Error(codes.ResourceExhausted, insufficientMessage)
	}
	return viber.GameplayError(err)
}
