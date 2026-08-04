package update

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/vertesan/solis-server/internal/crypto"
	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/octo"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/rich"
	"os"
	"path"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"golang.org/x/sync/semaphore"
)

func DataUpdateCheck() error {
	serverProfile, err := opsolis.FindServerProfile(context.Background())
	if err != nil {
		return err
	}
	rich.Info("server profile master version: %s", serverProfile.MasterVersion)

	masterTag, err := utils.ReadJsonFile[ptransaction.MasterTag](path.Join(config.GetManifestDir(), "mastertag.json"))
	if err != nil {
		return fmt.Errorf("failed to unmarshal masterTag: %w", err)
	}

	// masterTagPack
	if serverProfile.MasterVersion != masterTag.Version {
		rich.Info("mastertag json version: %s", masterTag.Version)
		// download encrypted database files from official server
		rich.Info("download master files from official server...")
		downloadAllEncryptedDb(masterTag)
		rich.Info("download completed")

		// upsert all masterTagPacks
		err := transaction.WithTransaction(func(ctx context.Context) (any, error) {
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
		rich.Info("masterTagPack collection update completed")
	}

	// refresh serverProfile after updating
	serverProfile, err = opsolis.FindServerProfile(context.Background())
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
	serverManifest, err := opsolis.FindManifestMetaInfo(context.Background())
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			serverManifest = &smodel.OctoManifest{Revision: 0}
		} else {
			return fmt.Errorf("failed to read octoManifest from database: %w", err)
		}
	}
	rich.Info("server octoManifest revision: %d", serverManifest.Revision)
	if octodb.Revision != serverManifest.Revision {
		rich.Info("OctoManifest json revision: %d", octodb.Revision)
		// replace all octoManifest
		err := transaction.WithTransaction(func(ctx context.Context) (any, error) {
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
		rich.Info("octoManifest collection update completed")
	}

	return nil
}

func downloadAllEncryptedDb(masterTag *ptransaction.MasterTag) error {
	var weight int64 = 10
	sem := semaphore.NewWeighted(weight)
	ctx := context.Background()
	for _, pack := range masterTag.MasterTagPacks {
		if err := sem.Acquire(ctx, 1); err != nil {
			return err
		}
		go downloadMasterTagPack(sem, pack)
	}
	if err := sem.Acquire(ctx, weight); err != nil {
		return err
	}
	return nil
}

func downloadMasterTagPack(sem *semaphore.Weighted, pack *ptransaction.MasterTagPack) {
	defer sem.Release(1)
	filePath := path.Join(config.GetDBDir(), pack.FileName)
	_, err := os.Stat(filePath)
	// file already exists
	if err == nil {
		return
	}
	if !os.IsNotExist(err) {
		rich.Fatal("failed to check master database file '%s': %v", pack.Type, err)
	}

	for i := 0; i < 3; i++ {
		resp, err := client.Get(pack.DownloadUrl)
		if err != nil {
			rich.Error("failed to download '%s': %v", pack.DownloadUrl, err)
			continue
		}
		defer resp.Close()
		if resp.StatusCode() != fiber.StatusOK {
			rich.Error("failed to download '%s': status %d", pack.DownloadUrl, resp.StatusCode())
			continue
		}
		fs, err := os.Create(filePath)
		if err != nil {
			rich.Fatal("failed to open path '%s': %v", filePath, err)
		}
		defer fs.Close()
		bufw := bufio.NewWriter(fs)
		if _, err := bufw.ReadFrom(resp.BodyStream()); err != nil {
			rich.Error("failed to read body from '%v'", pack.DownloadUrl)
			continue
		}
		if err := bufw.Flush(); err != nil {
			rich.Fatal("failed to flush body from '%v'", pack.DownloadUrl)
		}
		return
	}
	rich.Fatal("max retry exhausted when downloading '%s'", pack.DownloadUrl)
}
