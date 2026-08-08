package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// DeleteAllUserData removes every document owned by suid from the registered
// user collections, relying on the caller's transaction for all-or-nothing deletion.
func DeleteAllUserData(ctx context.Context, suid string) error {
	for _, collection := range uname.Collections {
		if err := ucommon.Delete(
			ctx,
			collection.Name,
			bson.M{collection.SUIDField: suid},
		); err != nil {
			return err
		}
	}
	return nil
}
