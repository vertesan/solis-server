package test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	mversion "github.com/vertesan/solis-server/internal/db/store/opmaster/version"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

// This startup scenario deliberately replaces one real master collection, so
// it is opt-in and refuses to run alongside a game server. All raw BSON is
// restored; encrypted fixture files and manifests exist only in t.TempDir().
func TestMasterOfficialUpdateScenario(t *testing.T) {
	if os.Getenv("MAKINO_TEST_MASTER_UPDATE") != "1" {
		t.Skip("set MAKINO_TEST_MASTER_UPDATE=1 with the game server stopped")
	}
	if testConfig.Env != Dev {
		t.Fatal("official master update scenario requires local development configuration")
	}
	if connection, err := net.DialTimeout("tcp", testConfig.GrpcFqdn, 200*time.Millisecond); err == nil {
		connection.Close()
		t.Fatal("stop the existing game server before running this startup scenario")
	}
	backupMakinoCatalogCollections(t, sname.ServerProfile, sname.MasterTagPack, sname.MasterTagPackOrig)
	backupMasterOfficialCollections(t, "Card", "_version")
	resourceDir := t.TempDir()
	t.Setenv("SOLIS_RESOURCE_DIR", resourceDir)
	for _, directory := range []string{config.GetDBDir(), config.GetManifestDir()} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	official := masterOfficialFixture(t, "official-1", "Original")
	writeMasterOfficialManifest(t, official)
	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		database := mongo.SessionFromContext(ctx).Client().Database("solis")
		for _, name := range []string{sname.MasterTagPack, sname.MasterTagPackOrig} {
			if _, err := database.Collection(name).DeleteMany(ctx, bson.M{}); err != nil {
				return nil, err
			}
		}
		if err := opsolis.UpsertMasterTagPack(ctx, official.MasterTagPacks[0]); err != nil {
			return nil, err
		}
		if _, err := database.Collection(sname.ServerProfile).UpdateOne(ctx, bson.M{}, bson.M{
			"$set": bson.M{"masterVersion": official.Version}, "$unset": bson.M{"masterVersionOrig": ""},
		}); err != nil {
			return nil, err
		}
		return nil, mversion.UpsertMasterVersion(ctx, official.Version)
	}); err != nil {
		t.Fatal(err)
	}
	// A normal restart does not invent the first backup or alter a matching
	// active version. The administrator deliberately triggers the update next.
	if err := db.CheckMasterDataUpdate(ctx); err != nil {
		t.Fatal(err)
	}
	profile, err := opsolis.FindServerProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	originals, err := opsolis.GetAllOriginalMasterTagPack(ctx)
	if err != nil || profile.MasterVersionOrig != "" || len(originals) != 0 {
		t.Fatalf("restart unexpectedly bootstrapped original manifests: %v", err)
	}
	if err := opsolis.UpdateServerProfileMasterVersion(ctx, "manual-refresh"); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckMasterDataUpdate(ctx); err != nil {
		t.Fatal(err)
	}
	assertMasterOfficialState(t, official, official, "Original")

	custom := masterOfficialFixture(t, "custom-1", "Customized")
	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := opsolis.UpsertMasterTagPack(ctx, custom.MasterTagPacks[0]); err != nil {
			return nil, err
		}
		return nil, opsolis.UpdateServerProfileMasterVersion(ctx, custom.Version)
	}); err != nil {
		t.Fatal(err)
	}
	// A stale master._version rehydrates from ACTIVE custom files, not the
	// original official file. Subsequent restarts leave this state unchanged.
	for range 2 {
		if err := db.CheckMasterDataUpdate(ctx); err != nil {
			t.Fatal(err)
		}
		assertMasterOfficialState(t, custom, official, "Customized")
	}

	next := masterOfficialFixture(t, "official-2", "New official")
	writeMasterOfficialManifest(t, next)
	// Obsolete backup entries must not survive a clean official refresh.
	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		_, err := mongo.SessionFromContext(ctx).Client().Database("solis").Collection(sname.MasterTagPackOrig).
			InsertOne(ctx, bson.M{"type": "RetiredOriginalOnly", "fileName": "obsolete"})
		return nil, err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckMasterDataUpdate(ctx); err != nil {
		t.Fatal(err)
	}
	assertMasterOfficialState(t, next, next, "New official")

	// A later pack failing to decrypt must roll back earlier collection writes
	// together with both manifests, both profile versions, and master._version.
	broken := masterOfficialFixture(t, "official-broken", "Must roll back")
	corrupt := &ptransaction.MasterTagPack{
		Type: "Character", FileName: strings.Repeat("e", 64), FileSize: 4096,
		CryptoKey: strings.Repeat("01", 32), DownloadUrl: "https://example.invalid/never-download",
	}
	if err := os.WriteFile(filepath.Join(config.GetDBDir(), corrupt.FileName), make([]byte, corrupt.FileSize), 0o600); err != nil {
		t.Fatal(err)
	}
	broken.MasterTagPacks = append(broken.MasterTagPacks, corrupt)
	writeMasterOfficialManifest(t, broken)
	if err := db.CheckMasterDataUpdate(ctx); err == nil {
		t.Fatal("invalid official database was accepted")
	}
	assertMasterOfficialState(t, next, next, "New official")
}

func masterOfficialFixture(t *testing.T, version, name string) *ptransaction.MasterTag {
	t.Helper()
	key := strings.Repeat("01", 32)
	filename := filepath.Join(config.GetDBDir(), version+".staging")
	database, err := sql.Open("sqlite3", filename+"?_pragma_key=x'"+key+"'")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`CREATE TABLE "Card" ("Id" TEXT PRIMARY KEY NOT NULL, "data" BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	data, err := proto.Marshal(&pmaster.Card{Id: "master-update-fixture", Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO "Card" ("Id", "data") VALUES (?, ?)`, "master-update-fixture", data); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(bytes)
	pack := &ptransaction.MasterTagPack{
		Type: "Card", FileName: hex.EncodeToString(hash[:]), FileSize: int32(len(bytes)), CryptoKey: key,
		DownloadUrl: "https://example.invalid/never-download",
	}
	if err := os.Rename(filename, filepath.Join(config.GetDBDir(), pack.FileName)); err != nil {
		t.Fatal(err)
	}
	return &ptransaction.MasterTag{Version: version, MasterTagPacks: []*ptransaction.MasterTagPack{pack}}
}

func writeMasterOfficialManifest(t *testing.T, tag *ptransaction.MasterTag) {
	t.Helper()
	data, err := json.Marshal(tag)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.GetManifestDir(), "mastertag.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertMasterOfficialState(t *testing.T, active, original *ptransaction.MasterTag, cardName string) {
	t.Helper()
	ctx := context.Background()
	profile, err := opsolis.FindServerProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if profile.MasterVersion != active.Version || profile.MasterVersionOrig != original.Version {
		t.Fatalf("unexpected active/original profile versions: %q / %q", profile.MasterVersion, profile.MasterVersionOrig)
	}
	version, err := mversion.GetMasterVersion(ctx)
	if err != nil || version != active.Version {
		t.Fatalf("master._version not consistent with publication: %q (%v)", version, err)
	}
	for _, check := range []struct {
		name string
		get  func(context.Context) ([]*ptransaction.MasterTagPack, error)
		want *ptransaction.MasterTag
	}{
		{"active", opsolis.GetAllMasterTagPack, active},
		{"original", opsolis.GetAllOriginalMasterTagPack, original},
	} {
		packs, err := check.get(ctx)
		if err != nil || len(packs) != 1 || !proto.Equal(packs[0], check.want.MasterTagPacks[0]) {
			t.Fatalf("%s manifest differs from expected publication (%v)", check.name, err)
		}
	}
	card, err := opmaster.FindCard(ctx, "master-update-fixture")
	if err != nil || card.GetName() != cardName {
		t.Fatalf("wrong published card: %q (%v)", card.GetName(), err)
	}
}

func backupMasterOfficialCollections(t *testing.T, names ...string) {
	t.Helper()
	snapshots := make(map[string][]bson.Raw, len(names))
	var client *mongo.Client
	err := transaction.WithTransaction(context.Background(), func(ctx context.Context) (any, error) {
		client = mongo.SessionFromContext(ctx).Client()
		database := client.Database("master")
		for _, name := range names {
			cursor, err := database.Collection(name).Find(ctx, bson.M{})
			if err != nil {
				return nil, err
			}
			var documents []bson.Raw
			if err := cursor.All(ctx, &documents); err != nil {
				return nil, err
			}
			snapshots[name] = documents
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("snapshot master collections: %v", err)
	}
	indexNames := make(map[string]map[string]bool, len(names))
	for _, name := range names {
		cursor, err := client.Database("master").Collection(name).Indexes().List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var indexes []struct {
			Name string `bson:"name"`
		}
		if err := cursor.All(context.Background(), &indexes); err != nil {
			t.Fatal(err)
		}
		indexNames[name] = make(map[string]bool, len(indexes))
		for _, index := range indexes {
			indexNames[name][index.Name] = true
		}
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
			database := mongo.SessionFromContext(ctx).Client().Database("master")
			for _, name := range names {
				collection := database.Collection(name)
				if _, err := collection.DeleteMany(ctx, bson.M{}); err != nil {
					return nil, err
				}
				if documents := snapshots[name]; len(documents) > 0 {
					if _, err := collection.InsertMany(ctx, documents); err != nil {
						return nil, err
					}
				}
			}
			return nil, nil
		})
		if err != nil {
			t.Error(fmt.Errorf("restore master collections: %w", err))
		}
		for _, name := range names {
			collection := client.Database("master").Collection(name)
			cursor, err := collection.Indexes().List(ctx)
			if err != nil {
				t.Error(err)
				continue
			}
			var indexes []struct {
				Name string `bson:"name"`
			}
			if err := cursor.All(ctx, &indexes); err != nil {
				t.Error(err)
				continue
			}
			for _, index := range indexes {
				if !indexNames[name][index.Name] && index.Name != "_id_" {
					if err := collection.Indexes().DropOne(ctx, index.Name); err != nil {
						t.Errorf("remove test-created index %s.%s: %v", name, index.Name, err)
					}
				}
			}
		}
	})
}
