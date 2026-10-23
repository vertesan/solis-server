package game

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/octo"
	"github.com/vertesan/solis-server/internal/server/depot"
	"github.com/vertesan/solis-server/internal/utils/crypto"

	"github.com/gofiber/fiber/v3"
	"google.golang.org/protobuf/proto"
)

var fixId = "205051"

var GetManifest fiber.Handler = func(c fiber.Ctx) error {
	someid := c.Params("someid")
	fromRevision := c.Params("fromv")

	octoKey := c.Get("X-OCTO-KEY")
	if octoKey != depot.ServerProfile.OctoApiKey {
		return fiber.NewError(403, "bad x-octo-key")
	}

	if someid != fixId {
		return fiber.NewError(404, fmt.Sprintf("manifest application ID %q does not match server ID %q", someid, fixId))
	}

	octoDatabase, manifestRevision, err := manifestResponse(c, fromRevision)
	if err != nil {
		return fiber.NewError(500, fmt.Sprintf("retrieve a consistent manifest: %v", err))
	}

	// marshal manifest
	octoBytes, err := proto.Marshal(octoDatabase)
	if err != nil {
		return fiber.NewError(500, fmt.Sprintf("marshal manifest protobuf: %v", err))
	}

	// encrypt manifest
	manifestKey, err := hex.DecodeString(depot.ServerProfile.OctoManifestKey)
	if err != nil {
		return fiber.NewError(500, fmt.Sprintf("decode manifest encryption key: %v", err))
	}
	buf, err := crypto.EncryptOctoManifest(bufio.NewReader(bytes.NewReader(octoBytes)), manifestKey)
	if err != nil {
		return fiber.NewError(500, fmt.Sprintf("encrypt manifest: %v", err))
	}

	// set response contents
	c.Append("Access-Control-Allow-Origin", "*")
	c.Append("Cache-Control", "public,max-age=0")
	c.Append("ETag", fmt.Sprintf("\"3.212.%s.%s.%d\"", fixId, fromRevision, manifestRevision))
	// c.Append("Content-Type", "application/x-protobuf")
	c.Append("Vary", "Accept")
	c.Append("Age", "0")
	c.RequestCtx().SetContentType("application/x-protobuf")
	return c.Send(buf)
}

// manifestResponse builds either an empty delta or the full manifest while a
// snapshot transaction keeps its metadata and body at the same revision.
func manifestResponse(ctx context.Context, fromRevision string) (*octo.Database, int32, error) {
	var response *octo.Database
	var revision int32
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		manifestMetaInfo, err := opsolis.FindManifestMetaInfo(ctx)
		if err != nil {
			return nil, err
		}
		revision = manifestMetaInfo.Revision
		if fromRevision == strconv.Itoa(int(revision)) {
			response = &octo.Database{
				Revision:        revision,
				Tagname:         []string{},
				AssetBundleList: []*octo.Data{},
				ResourceList:    []*octo.Data{},
				UrlFormat:       manifestMetaInfo.UrlFormat,
			}
			return response, nil
		}
		response, err = opsolis.FindFullManifest(ctx)
		return response, err
	})
	if err != nil {
		return nil, 0, err
	}
	return response, revision, nil
}
