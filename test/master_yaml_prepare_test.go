package test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	mastercrypto "github.com/vertesan/solis-server/internal/utils/crypto"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

func TestMasterPrepareImportScenario(t *testing.T) {
	if os.Getenv("MAKINO_TEST_MASTER_UPDATE") != "1" {
		t.Skip("set MAKINO_TEST_MASTER_UPDATE=1 with the game server stopped")
	}
	if testConfig.Env != Dev {
		t.Fatal("master preparation scenario requires local development configuration")
	}
	if connection, err := net.DialTimeout("tcp", testConfig.GrpcFqdn, 200*time.Millisecond); err == nil {
		connection.Close()
		t.Fatal("stop the existing game server before running master preparation")
	}
	backupMakinoCatalogCollections(t, sname.ServerProfile, sname.MasterTagPack, sname.MasterTagPackOrig)
	backupMasterOfficialCollections(t, "Card", "LoveLevel", "_version")
	t.Setenv("SOLIS_RESOURCE_DIR", t.TempDir())
	for _, directory := range []string{config.GetDBDir(), config.GetManifestDir()} {
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	tag := masterOfficialFixture(t, "prepare-import-official", "Original")
	tag.MasterTagPacks = append(tag.MasterTagPacks, masterPrepareLoveFixture(t))
	writeMasterOfficialManifest(t, tag)
	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		database := mongo.SessionFromContext(ctx).Client().Database("solis")
		for _, collection := range []string{sname.MasterTagPack, sname.MasterTagPackOrig} {
			if _, err := database.Collection(collection).DeleteMany(ctx, bson.M{}); err != nil {
				return nil, err
			}
		}
		for _, pack := range tag.MasterTagPacks {
			if err := opsolis.UpsertMasterTagPack(ctx, pack); err != nil {
				return nil, err
			}
		}
		_, err := database.Collection(sname.ServerProfile).UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"masterVersion": "prepare-manual-refresh"}, "$unset": bson.M{"masterVersionOrig": ""}})
		return nil, err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckMasterDataUpdate(ctx); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := masterdata.Export(ctx, masterdata.ExportOptions{All: true, Directory: directory}); err != nil {
		t.Fatal(err)
	}
	cardPath, lovePath := filepath.Join(directory, "Card.yaml"), filepath.Join(directory, "LoveLevel.yaml")
	cardOriginal, err := os.ReadFile(cardPath)
	if err != nil {
		t.Fatal(err)
	}
	loveOriginal, err := os.ReadFile(lovePath)
	if err != nil {
		t.Fatal(err)
	}
	prepare := func(files []string, all, allowEmpty bool) *masterdata.ImportRequest {
		t.Helper()
		request, err := masterdata.PrepareImport(ctx, masterdata.ImportOptions{Files: files, All: all, AllowEmpty: allowEmpty})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := masterdata.RemoveStaging(request.ID); err != nil {
				t.Error(err)
			}
		})
		return request
	}
	reject := func(files []string, all bool) {
		t.Helper()
		if request, err := masterdata.PrepareImport(ctx, masterdata.ImportOptions{Files: files, All: all}); err == nil {
			masterdata.RemoveStaging(request.ID)
			t.Fatal("invalid master import accepted")
		}
	}
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if request := prepare([]string{cardPath, lovePath}, true, false); len(request.Packs) != 0 {
		t.Fatal("unchanged YAML generated a publication")
	}
	reject([]string{cardPath}, true)
	reject([]string{cardPath, cardPath}, false)
	write(cardPath, []byte(strings.Replace(string(cardOriginal), tag.Version, "stale-version", 1)))
	reject([]string{cardPath}, false)
	write(cardPath, cardOriginal)
	cardHeader := strings.SplitN(string(cardOriginal), "---", 2)[0] + "---\n"
	write(cardPath, []byte(cardHeader+"data: {id: replacement-card, name: Changed, releaseDate: 9223372036854775807}\n"))
	request := prepare([]string{cardPath}, false, false)
	if len(request.Packs) != 1 || request.Packs[0].RowCount != 1 {
		t.Fatal("changed collection was not staged")
	}
	stagedPath, err := masterdata.StagedPath(config.GetDBDir(), request.ID, request.Packs[0].FileName)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := mastercrypto.DecryptMasterDbFile("Card", stagedPath, tag.MasterTagPacks[0].CryptoKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["Id"] != "replacement-card" {
		t.Fatalf("fresh file retained deleted original records: %d rows", len(rows))
	}
	write(cardPath, []byte(cardHeader+"data: {id: same-card}\n---\ndata: {id: same-card, name: Duplicate}\n"))
	reject([]string{cardPath}, false)
	write(cardPath, []byte(strings.TrimSuffix(cardHeader, "---\n")))
	reject([]string{cardPath}, false)
	if request := prepare([]string{cardPath}, false, true); len(request.Packs) != 1 || request.Packs[0].RowCount != 0 {
		t.Fatal("explicit empty-table import failed")
	}
	write(cardPath, cardOriginal)
	loveHeader := strings.SplitN(string(loveOriginal), "---", 2)[0] + "---\n"
	write(lovePath, []byte(loveHeader+"data: {loveId: love-fixture, level: 1, requiredExp: 9007199254740993}\n---\ndata: {loveId: love-fixture, level: 2, requiredExp: 9223372036854775807}\n"))
	if request := prepare([]string{lovePath}, false, false); len(request.Packs) != 1 || request.Packs[0].RowCount != 2 {
		t.Fatal("composite key insertion failed")
	}
	write(lovePath, []byte(loveHeader+"data: {loveId: love-fixture, level: 1}\n---\ndata: {loveId: love-fixture, level: 1, requiredExp: 2}\n"))
	reject([]string{lovePath}, false)
	write(lovePath, loveOriginal)
	assertMasterOfficialStateForPreparation(t, tag)
}

func masterPrepareLoveFixture(t *testing.T) *ptransaction.MasterTagPack {
	t.Helper()
	key := strings.Repeat("01", 32)
	path := filepath.Join(config.GetDBDir(), "love-fixture.building")
	database, err := sql.Open("sqlite3", path+"?_pragma_key=x'"+key+"'")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`CREATE TABLE "LoveLevel" ("LoveId" TEXT NOT NULL, "Level" INTEGER NOT NULL, "data" BLOB NOT NULL, PRIMARY KEY ("LoveId","Level"))`); err != nil {
		t.Fatal(err)
	}
	data, err := proto.Marshal(&pmaster.LoveLevel{LoveId: "love-fixture", Level: 1, RequiredExp: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO "LoveLevel" VALUES (?,?,?)`, "love-fixture", 1, data); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	name := hex.EncodeToString(digest[:])
	if err := os.Rename(path, filepath.Join(config.GetDBDir(), name)); err != nil {
		t.Fatal(err)
	}
	return &ptransaction.MasterTagPack{Type: "LoveLevel", FileName: name, FileSize: int32(len(contents)), CryptoKey: key, DownloadUrl: "https://example.invalid/never-download"}
}

func assertMasterOfficialStateForPreparation(t *testing.T, tag *ptransaction.MasterTag) {
	t.Helper()
	ctx := context.Background()
	profile, err := opsolis.FindServerProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if profile.MasterVersion != tag.Version || profile.MasterVersionOrig != tag.Version {
		t.Fatal("staging mutated versions")
	}
	packs, err := opsolis.GetAllMasterTagPack(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != len(tag.MasterTagPacks) {
		t.Fatal("staging mutated active manifest")
	}
	for _, pack := range packs {
		matched := false
		for _, want := range tag.MasterTagPacks {
			if proto.Equal(pack, want) {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatal("staging changed active manifest metadata")
		}
	}
}
