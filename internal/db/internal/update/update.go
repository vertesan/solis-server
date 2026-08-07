package update

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/octo"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/crypto"
	"os"
	"path"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func DataUpdateCheck() error {
	ctx := context.Background()
	serverProfile, err := opsolis.FindServerProfile(ctx)
	if err != nil {
		return err
	}
	zap.L().Sugar().Infof("server profile master version: %s", serverProfile.MasterVersion)

	masterTag, err := utils.ReadJsonFile[ptransaction.MasterTag](path.Join(config.GetManifestDir(), "mastertag.json"))
	if err != nil {
		return fmt.Errorf("failed to unmarshal masterTag: %w", err)
	}

	// masterTagPack
	if serverProfile.MasterVersion != masterTag.Version {
		zap.L().Sugar().Infof("mastertag json version: %s", masterTag.Version)
		// download encrypted database files from official server
		zap.L().Info("download master files from official server...")
		if err := downloadAllEncryptedDb(masterTag); err != nil {
			return fmt.Errorf("failed to download master files: %w", err)
		}
		zap.L().Info("download completed")

		// upsert all masterTagPacks
		err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
			for _, masterTagPack := range masterTag.MasterTagPacks {
				if err := opsolis.UpsertMasterTagPack(ctx, masterTagPack); err != nil {
					return nil, err
				}
			}
			if err := opsolis.UpdateServerProfileMasterVersion(ctx, masterTag.Version); err != nil {
				return nil, err
			}
			return nil, nil
		}, readconcern.Local(), writeconcern.Majority())
		if err != nil {
			return fmt.Errorf("failed to upsert masterTagPack: %w", err)
		}
		zap.L().Info("masterTagPack collection update completed")
	}

	// refresh serverProfile after updating
	serverProfile, err = opsolis.FindServerProfile(ctx)
	if err != nil {
		return err
	}

	if err := crypto.CheckAndUpdateAllMaster(serverProfile.MasterVersion); err != nil {
		return fmt.Errorf("failed to update master database: %w", err)
	}

	// octoManifest
	octodb, err := utils.ReadJsonFile[octo.Database](path.Join(config.GetManifestDir(), "OctoManifest.json"))
	if err != nil {
		return fmt.Errorf("failed to unmarshal octo.Database: %w", err)
	}
	serverManifest, err := opsolis.FindManifestMetaInfo(ctx)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			serverManifest = &smodel.OctoManifest{Revision: 0}
		} else {
			return fmt.Errorf("failed to read octoManifest from database: %w", err)
		}
	}
	zap.L().Sugar().Infof("server octoManifest revision: %d", serverManifest.Revision)
	if octodb.Revision != serverManifest.Revision {
		zap.L().Sugar().Infof("OctoManifest json revision: %d", octodb.Revision)
		// replace all octoManifest
		err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
			if err := opsolis.DeleteAllManifests(ctx); err != nil {
				return nil, err
			}
			if err := opsolis.InsertManifest(ctx, octodb); err != nil {
				return nil, err
			}
			return nil, nil
		}, readconcern.Local(), writeconcern.W1())
		if err != nil {
			return fmt.Errorf("failed to replace octoManifest: %w", err)
		}
		zap.L().Info("octoManifest collection update completed")
	}

	return nil
}

func downloadAllEncryptedDb(masterTag *ptransaction.MasterTag) error {
	group := new(errgroup.Group)
	group.SetLimit(10)
	for _, pack := range masterTag.MasterTagPacks {
		group.Go(func() error {
			return downloadMasterTagPack(pack)
		})
	}
	return group.Wait()
}

func downloadMasterTagPack(pack *ptransaction.MasterTagPack) error {
	filePath := path.Join(config.GetDBDir(), pack.FileName)
	_, err := os.Stat(filePath)
	// file already exists
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check master database file %q: %w", pack.Type, err)
	}

	var lastErr error
	for i := 0; i < 3; i++ {
		if err := downloadMasterTagPackAttempt(filePath, pack); err != nil {
			lastErr = err
			zap.L().Sugar().Warnf("failed to download '%s': %v", pack.DownloadUrl, err)
			continue
		}
		return nil
	}
	return fmt.Errorf("max retries exhausted when downloading %q: %w", pack.DownloadUrl, lastErr)
}

func downloadMasterTagPackAttempt(filePath string, pack *ptransaction.MasterTagPack) error {
	resp, err := client.Get(pack.DownloadUrl)
	if err != nil {
		return err
	}
	defer resp.Close()
	if resp.StatusCode() != fiber.StatusOK {
		return fmt.Errorf("unexpected response status %d", resp.StatusCode())
	}

	fs, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to open path %q: %w", filePath, err)
	}
	defer fs.Close()

	bufw := bufio.NewWriter(fs)
	if _, err := bufw.ReadFrom(resp.BodyStream()); err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	if err := bufw.Flush(); err != nil {
		return fmt.Errorf("failed to flush response body: %w", err)
	}
	return nil
}
