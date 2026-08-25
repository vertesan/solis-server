package opuser

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

var ErrDuplicateAppRequest = errors.New("duplicate app request")

// ReserveAppRequest records requestId before an action is handled. MongoDB's
// unique _id index makes the reservation atomic across server processes.
func ReserveAppRequest(ctx context.Context, suid, requestId, method string) error {
	request := &umodel.UserAppRequest{
		Key:        suid + ":" + requestId,
		SUID:       suid,
		RequestId:  requestId,
		Method:     method,
		ReceivedAt: time.Now().UnixMilli(),
	}
	if err := ucommon.InsertOne(ctx, uname.UserAppRequest, request); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicateAppRequest
		}
		return fmt.Errorf("failed to reserve app request: %w", err)
	}
	return nil
}
