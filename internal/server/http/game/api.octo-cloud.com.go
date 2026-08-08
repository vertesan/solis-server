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
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.uber.org/zap"

	"github.com/gofiber/fiber/v3"
	"google.golang.org/protobuf/proto"
)

var fixId = "205051"

var GetManifest fiber.Handler = func(c fiber.Ctx) error {
	logger := mcontext.GetLogger(c)
	someid := c.Params("someid")
	fromRevision := c.Params("fromv")

	octoKey := c.Get("X-OCTO-KEY")
	if octoKey != depot.ServerProfile.OctoApiKey {
		logger.Sugar().Warnf("client octo key header mismatched, gives %q", octoKey)
		return fiber.NewError(403, "bad x-octo-key")
	}

	if someid != fixId {
		logger.Sugar().Errorf("client requested id %q, mismatched with current server fix id %q", someid, fixId)
		return fiber.NewError(404, "not found")
	}

	octoDatabase, manifestRevision, err := manifestResponse(c, fromRevision)
	if err != nil {
		logger.Error("failed to retrieve a consistent manifest", zap.Error(err))
		return fiber.NewError(500, "internal error")
	}

	// marshal manifest
	octoBytes, err := proto.Marshal(octoDatabase)
	if err != nil {
		logger.Error("failed converting octoDatabase from protobuf object to bytes", zap.Error(err))
		return fiber.NewError(500, "internal error")
	}

	// encrypt manifest
	manifestKey, err := hex.DecodeString(depot.ServerProfile.OctoManifestKey)
	if err != nil {
		logger.Error("failed converting manifest key hex string to byte slice", zap.Error(err))
		return fiber.NewError(500, "internal error")
	}
	buf, err := crypto.EncryptOctoManifest(bufio.NewReader(bytes.NewReader(octoBytes)), manifestKey)
	if err != nil {
		return fiber.NewError(500, "internal error")
	}

	// set response contents
	c.Append("Access-Control-Allow-Origin", "*")
	c.Append("Cache-Control", "public,max-age=0")
	c.Append("ETag", fmt.Sprintf("\"3.212.%s.%s.%d\"", fixId, fromRevision, manifestRevision))
	// c.Append("Content-Type", "application/x-protobuf")
	c.Append("Vary", "Accept")
	c.Append("Age", "0")
	c.RequestCtx().SetContentType("application/x-protobuf")
	err = c.Send(buf)
	if err != nil {
		logger.Error("failed send back octoBytes to client", zap.Error(err))
	}
	return err
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
