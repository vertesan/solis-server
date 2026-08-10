package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	mversion "github.com/vertesan/solis-server/internal/db/store/opmaster/version"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/octo"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/crypto"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// check and update both master data and octo manifest data
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

		// upsert all masterTagPacks and master version for serverProfile
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
		})
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

	if err := checkAndUpdateAllMaster(serverProfile.MasterVersion); err != nil {
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
		err := transaction.WithTransactionDangerously(ctx, func(ctx context.Context) (any, error) {
			if err := opsolis.DeleteAllManifests(ctx); err != nil {
				return nil, err
			}
			if err := opsolis.InsertManifest(ctx, octodb); err != nil {
				return nil, err
			}
			return nil, nil
		})
		if err != nil {
			return fmt.Errorf("failed to replace octoManifest: %w", err)
		}
		zap.L().Info("octoManifest collection update completed")
	}

	return nil
}

func checkAndUpdateAllMaster(profileVersion string) error {
	ctx := context.Background()
	ver, err := mversion.GetMasterVersion(ctx)
	if err != nil {
		return err
	}
	zap.L().Sugar().Infof("master database version %s", ver)
	// check current version vs version string in serverProfile
	if profileVersion != "" && ver == profileVersion {
		return nil
	}
	zap.L().Info("updating master database from storage")
	// get all masterTagPacks
	tags, err := opsolis.GetAllMasterTagPack(ctx)
	if err != nil {
		return fmt.Errorf("failed to get master tag pack: %w", err)
	}
	// prepare an indexMap for creating indexes after transaction
	indexMap := make(map[string]bson.D)

	// start a transaction to migrate master databases from SQLite to MongoDB
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (interface{}, error) {
		for _, tag := range tags {
			filePath := path.Join(config.GetDBDir(), tag.FileName)
			data, err := crypto.DecryptMasterDbFile(tag.Type, filePath, tag.CryptoKey)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt db: %w", err)
			}
			// create indexMap
			if len(data) > 0 {
				var indexes bson.D
				for key := range data[0] {
					if key != "data" {
						indexes = append(indexes, bson.E{Key: key, Value: 1})
					}
				}
				indexMap[tag.Type] = indexes
			}
			// update db
			if err := mversion.DeleteMasterDb(ctx, tag.Type); err != nil {
				return nil, err
			}
			if err := mversion.InsertMasterDb(ctx, tag.Type, data); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("failed to update master db: %w", err)
	}

	// creating indexes is not allowed in a transaction, we have to do this separately
	zap.L().Info("creating indexes for master database, this may take a few minutes...")
	for k, v := range indexMap {
		if err := mversion.CreateIndexToMasterDb(ctx, k, v); err != nil {
			return err
		}
	}

	// finally, upsert master version in master._version
	if err := mversion.UpsertMasterVersion(ctx, profileVersion); err != nil {
		return err
	}

	zap.L().Info("master database update completed")
	return nil
}

func downloadAllEncryptedDb(masterTag *ptransaction.MasterTag) error {
	if err := validateMasterTagPacksForDownload(masterTag); err != nil {
		return err
	}
	group := new(errgroup.Group)
	group.SetLimit(10)
	for _, pack := range masterTag.MasterTagPacks {
		group.Go(func() error {
			return downloadMasterTagPack(pack)
		})
	}
	return group.Wait()
}

// validateMasterTagPacksForDownload checks all metadata needed to download pack
// files safely before any concurrent downloads begin.
func validateMasterTagPacksForDownload(masterTag *ptransaction.MasterTag) error {
	if masterTag == nil {
		return fmt.Errorf("master tag is nil")
	}
	if masterTag.Version == "" {
		return fmt.Errorf("master tag version is empty")
	}
	if len(masterTag.MasterTagPacks) == 0 {
		return fmt.Errorf("master tag contains no packs")
	}
	types := make(map[string]struct{}, len(masterTag.MasterTagPacks))
	fileNames := make(map[string]struct{}, len(masterTag.MasterTagPacks))
	for i, pack := range masterTag.MasterTagPacks {
		if pack == nil {
			return fmt.Errorf("master tag pack at index %d is nil", i)
		}
		if pack.Type == "" {
			return fmt.Errorf("master tag pack at index %d has an empty type", i)
		}
		if _, exists := types[pack.Type]; exists {
			return fmt.Errorf("duplicate master tag pack type %q", pack.Type)
		}
		types[pack.Type] = struct{}{}
		if pack.FileName == "" || path.Base(pack.FileName) != pack.FileName {
			return fmt.Errorf("master tag pack %q has an invalid file name %q", pack.Type, pack.FileName)
		}
		if _, exists := fileNames[pack.FileName]; exists {
			return fmt.Errorf("duplicate master tag pack file name %q", pack.FileName)
		}
		fileNames[pack.FileName] = struct{}{}
		if pack.FileSize <= 0 {
			return fmt.Errorf("master tag pack %q has an invalid file size %d", pack.Type, pack.FileSize)
		}
		if pack.DownloadUrl == "" {
			return fmt.Errorf("master tag pack %q has an empty download URL", pack.Type)
		}
		if pack.CryptoKey == "" {
			return fmt.Errorf("master tag pack %q has an empty crypto key", pack.Type)
		}
	}
	return nil
}

func downloadMasterTagPack(pack *ptransaction.MasterTagPack) error {
	if pack == nil {
		return fmt.Errorf("master tag pack is nil")
	}
	filePath := path.Join(config.GetDBDir(), pack.FileName)
	info, err := os.Stat(filePath)
	// A complete file already exists.
	if err == nil {
		if info.Mode().IsRegular() && info.Size() == int64(pack.FileSize) {
			return nil
		}
		zap.L().Sugar().Warnf("replacing incomplete master database file %q", pack.Type)
	}
	if err != nil && !os.IsNotExist(err) {
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
	return publishMasterTagPack(filePath, pack, resp.BodyStream())
}

// publishMasterTagPack writes and synchronizes a complete pack to a temporary
// file, then atomically replaces the cached file at filePath.
func publishMasterTagPack(filePath string, pack *ptransaction.MasterTagPack, source io.Reader) error {
	fs, err := os.CreateTemp(path.Dir(filePath), "."+path.Base(filePath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary master database file: %w", err)
	}
	tempPath := fs.Name()
	closed := false
	renamed := false
	defer func() {
		if !closed {
			_ = fs.Close()
		}
		if !renamed {
			_ = os.Remove(tempPath)
		}
	}()

	written, err := io.Copy(fs, source)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	if written != int64(pack.FileSize) {
		return fmt.Errorf("unexpected master database size %d, want %d", written, pack.FileSize)
	}
	if err := fs.Chmod(0o644); err != nil {
		return fmt.Errorf("failed to set master database file permissions: %w", err)
	}
	if err := fs.Sync(); err != nil {
		return fmt.Errorf("failed to sync temporary master database file: %w", err)
	}
	if err := fs.Close(); err != nil {
		return fmt.Errorf("failed to close temporary master database file: %w", err)
	}
	closed = true
	if err := os.Rename(tempPath, filePath); err != nil {
		return fmt.Errorf("failed to publish master database file: %w", err)
	}
	renamed = true
	directory, err := os.Open(path.Dir(filePath))
	if err != nil {
		return fmt.Errorf("failed to open master database directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("failed to sync master database directory: %w", err)
	}
	return nil
}
