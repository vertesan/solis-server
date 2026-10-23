package test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	mversion "github.com/vertesan/solis-server/internal/db/store/opmaster/version"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/crypto"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// Customization tests use tiny encrypted packs under a temporary resource tree.
// Actual master documents, indexes, manifests and versions are restored by the
// existing backup helpers. No game users are created and no server is started.
func setupCustomMaster(t *testing.T) (*ptransaction.MasterTag, map[string][]proto.Message) {
	t.Helper()
	if os.Getenv("MAKINO_TEST_CUSTOM") != "1" {
		t.Skip("set MAKINO_TEST_CUSTOM=1 with the game server stopped")
	}
	if testConfig.Env != Dev {
		t.Fatal("custom master scenarios require local development configuration")
	}
	if connection, err := net.DialTimeout("tcp", testConfig.GrpcFqdn, 200*time.Millisecond); err == nil {
		connection.Close()
		t.Fatal("stop the game server before running isolated customization scenarios")
	}
	rows := map[string][]proto.Message{
		"HomeAction": {
			&pmaster.HomeAction{HomeActionId: "ordinary", CharacterId: "char-keep", Text: "Keep me", ConditionId: "ordinary-condition", Weight: 37},
			&pmaster.HomeAction{HomeActionId: "love-one", CharacterId: "old-character", Text: "Replace me", ConditionId: "old-condition", Weight: 7},
		},
		"LoveHomeAction": {
			&pmaster.LoveHomeAction{Id: "love-one", LoveId: "love-fixture", CharacterId: "char-rio", MotionAssetId: "motion", AdditionMotionAssetId: "additional-motion", VoiceAssetId: "voice", Text: "ありがとう。\nいつも。", HiddenFavorabilityGte: 7226, UnlockLoveStoryEndIds: []string{"end"}, Weight: 1000, ActionType: penum.LoveHomeActionType_LoveHomeActionType_Present, CostumeId: "cos-rio"},
			&pmaster.LoveHomeAction{Id: "love-two", LoveId: "love-extra-fixture", CharacterId: "char-maho", VoiceAssetId: "other-voice", Text: "Another action", Weight: 2000},
		},
		"Costume": {
			&pmaster.Costume{Id: "costume-one", CostumeTypeId: "casual"},
			&pmaster.Costume{Id: "costume-two", CostumeTypeId: "casual"},
			&pmaster.Costume{Id: "costume-sd", CostumeTypeId: "sd-ambassador", IsUserUnavailable: true},
			&pmaster.Costume{Id: "costume-empty"},
		},
		"PhotoActivity": {
			&pmaster.PhotoActivity{Id: "activity", Name: "Keep activity", StageId: "stage", CostumeTypeIds: []string{"old"}, ForceCostumeTypeId: "keep-force", ViewConditionId: "hide", UnlockConditionId: "keep-unlock", ImpossibleShootingCharacterIds: []string{"char-rio"}, Order: 13},
			&pmaster.PhotoActivity{Id: "activity-two", Name: "Also unlocked", CostumeTypeIds: []string{"old-two"}, ViewConditionId: "other-hide"},
		},
		"PhotoExpression": {
			&pmaster.PhotoExpression{Id: "expression", Name: "Keep expression", CostumeTypeIds: []string{"old"}, ForceCostumeTypeId: "keep-force", ViewConditionId: "hide", UnlockConditionId: "keep-unlock", ImpossibleShootingCharacterIds: []string{"char-rio"}, ImpossiblePhotoPoseIds: []string{"pose"}, SceneLayoutAssetId: "keep-layout", CharacterAmountArray: []int32{1, 3}},
		},
		"PhotoQuestMusic": {
			&pmaster.PhotoQuestMusic{Id: "music", Name: "Keep music", AssetId: "music-asset", Singer: "Singer", ViewConditionId: "hide", UnlockConditionId: "locked"},
		},
		"PhotoQuestStage": {
			&pmaster.PhotoQuestStage{Id: "stage", Name: "Keep stage", AssetId: "stage-asset", MaxCapacity: 5000, ViewConditionId: "hide", UnlockConditionId: "locked"},
		},
	}
	names := make([]string, 0, len(rows))
	for name := range rows {
		names = append(names, name)
	}
	sort.Strings(names)
	t.Cleanup(func() {
		if err := masterdata.RefreshSnapshot(context.Background()); err != nil {
			t.Error(err)
		}
	})
	backupMakinoCatalogCollections(t, sname.ServerProfile, sname.MasterTagPack, sname.MasterTagPackOrig, sname.MasterImportJob)
	backupMasterOfficialCollections(t, append(names, "_version")...)
	t.Setenv("SOLIS_RESOURCE_DIR", t.TempDir())
	if err := os.Mkdir(config.GetDBDir(), 0700); err != nil {
		t.Fatal(err)
	}
	original := &ptransaction.MasterTag{Version: "custom-template-original"}
	for _, name := range names {
		original.MasterTagPacks = append(original.MasterTagPacks, customMasterFixture(t, name, rows[name]))
	}
	ctx := context.Background()
	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := mongo.SessionFromContext(ctx).Client().Database("solis").Collection(sname.MasterImportJob).DeleteMany(ctx, bson.M{}); err != nil {
			return nil, err
		}
		if err := opsolis.ReplaceOfficialMasterTag(ctx, original); err != nil {
			return nil, err
		}
		for _, pack := range original.MasterTagPacks {
			data, err := crypto.DecryptMasterDbFile(pack.Type, filepath.Join(config.GetDBDir(), pack.FileName), pack.CryptoKey)
			if err != nil {
				return nil, err
			}
			if err := mversion.DeleteMasterDb(ctx, pack.Type); err != nil {
				return nil, err
			}
			if err := mversion.InsertMasterDb(ctx, pack.Type, data); err != nil {
				return nil, err
			}
		}
		return nil, mversion.UpsertMasterVersion(ctx, original.Version)
	}); err != nil {
		t.Fatal(err)
	}
	if err := masterdata.RefreshSnapshot(ctx); err != nil {
		t.Fatal(err)
	}
	return original, rows
}

func customMasterFixture(t *testing.T, name string, rows []proto.Message) *ptransaction.MasterTagPack {
	t.Helper()
	key := strings.Repeat("07", 32)
	path := filepath.Join(config.GetDBDir(), name+".staging")
	database, err := sql.Open("sqlite3", path+"?_pragma_key=x'"+key+"'")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	column := "Id"
	if name == "HomeAction" {
		column = "HomeActionId"
	}
	if _, err := database.Exec(fmt.Sprintf(`CREATE TABLE "%s" ("%s" TEXT PRIMARY KEY NOT NULL, "data" BLOB NOT NULL)`, name, column)); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		data, err := proto.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		message := row.ProtoReflect()
		id := message.Get(message.Descriptor().Fields().ByNumber(1)).String()
		if _, err := database.Exec(fmt.Sprintf(`INSERT INTO "%s" VALUES (?, ?)`, name), id, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(contents)
	pack := &ptransaction.MasterTagPack{Type: name, FileName: hex.EncodeToString(hash[:]), FileSize: int32(len(contents)), CryptoKey: key, DownloadUrl: "https://example.invalid/never-download"}
	if err := os.Rename(path, filepath.Join(config.GetDBDir(), pack.FileName)); err != nil {
		t.Fatal(err)
	}
	return pack
}

func assertCustomMasterRows(t *testing.T, name string, expected []proto.Message) {
	t.Helper()
	err := transaction.WithTransaction(context.Background(), func(ctx context.Context) (any, error) {
		cursor, err := mongo.SessionFromContext(ctx).Client().Database("master").Collection(name).Find(ctx, bson.M{})
		if err != nil {
			return nil, err
		}
		defer cursor.Close(ctx)
		count := 0
		for cursor.Next(ctx) {
			var record struct{ Data []byte }
			if err := cursor.Decode(&record); err != nil {
				return nil, err
			}
			found := false
			for _, want := range expected {
				got := want.ProtoReflect().Type().New().Interface()
				if err := proto.Unmarshal(record.Data, got); err != nil {
					return nil, err
				}
				found = found || proto.Equal(got, want)
			}
			if !found {
				return nil, fmt.Errorf("%s contains unexpected transformed row", name)
			}
			count++
		}
		if err := cursor.Err(); err != nil {
			return nil, err
		}
		if count != len(expected) {
			return nil, fmt.Errorf("%s has %d rows, expected %d", name, count, len(expected))
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertCustomMasterSources(t *testing.T, original *ptransaction.MasterTag, rows map[string][]proto.Message) {
	t.Helper()
	active := masterPublicationActive(t)
	for _, name := range []string{"LoveHomeAction", "Costume"} {
		assertCustomMasterRows(t, name, rows[name])
		var previous, current *ptransaction.MasterTagPack
		for _, pack := range original.MasterTagPacks {
			if pack.Type == name {
				previous = pack
			}
		}
		for _, pack := range active.MasterTagPacks {
			if pack.Type == name {
				current = pack
			}
		}
		if !proto.Equal(previous, current) {
			t.Fatalf("read-only source pack %s was changed", name)
		}
	}
	for _, pack := range original.MasterTagPacks {
		if _, err := os.Stat(filepath.Join(config.GetDBDir(), pack.FileName)); err != nil {
			t.Fatalf("original %s file was deleted: %v", pack.Type, err)
		}
	}
}

func customMasterExpectedPhoto(rows map[string][]proto.Message) map[string][]proto.Message {
	expected := make(map[string][]proto.Message)
	for _, message := range rows["PhotoActivity"] {
		value := proto.Clone(message).(*pmaster.PhotoActivity)
		value.ViewConditionId, value.ImpossibleShootingCharacterIds = "", nil
		value.CostumeTypeIds = []string{"casual", "sd-ambassador"}
		expected["PhotoActivity"] = append(expected["PhotoActivity"], value)
	}
	value := proto.Clone(rows["PhotoExpression"][0]).(*pmaster.PhotoExpression)
	value.ViewConditionId, value.ImpossibleShootingCharacterIds, value.ImpossiblePhotoPoseIds = "", nil, nil
	value.CostumeTypeIds = []string{"casual", "sd-ambassador"}
	expected["PhotoExpression"] = []proto.Message{value}
	music := proto.Clone(rows["PhotoQuestMusic"][0]).(*pmaster.PhotoQuestMusic)
	music.ViewConditionId, music.UnlockConditionId = "", ""
	expected["PhotoQuestMusic"] = []proto.Message{music}
	stage := proto.Clone(rows["PhotoQuestStage"][0]).(*pmaster.PhotoQuestStage)
	stage.ViewConditionId, stage.UnlockConditionId = "", ""
	expected["PhotoQuestStage"] = []proto.Message{stage}
	return expected
}

func customMasterExpectedHome(rows map[string][]proto.Message) []proto.Message {
	result := []proto.Message{rows["HomeAction"][0]}
	for _, message := range rows["LoveHomeAction"] {
		value := message.(*pmaster.LoveHomeAction)
		result = append(result, &pmaster.HomeAction{HomeActionId: value.Id, CharacterId: value.CharacterId, MotionAssetId: value.MotionAssetId,
			AdditionMotionAssetId: value.AdditionMotionAssetId, VoiceAssetId: value.VoiceAssetId, Text: value.Text, Weight: 100})
	}
	return result
}

func TestCustomMasterTemplatesScenario(t *testing.T) {
	original, rows := setupCustomMaster(t)
	ctx := context.Background()
	owner, err := masterdata.AcquireOwnership()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	publish := func(request *masterdata.ImportRequest) {
		t.Helper()
		job, err := owner.Publish(ctx, *request)
		if err != nil || job == nil || !job.Committed || job.State != "complete" {
			t.Fatalf("custom publication failed: %v", err)
		}
	}
	for _, template := range []string{"home-action", "photo"} {
		request, err := masterdata.PrepareCustom(ctx, template)
		if err != nil {
			t.Fatal(err)
		}
		names, err := masterdata.CustomTypes(template)
		if err != nil || len(request.Packs) != len(names) {
			t.Fatalf("incorrect %s target set: %v", template, err)
		}
		for _, pack := range request.Packs {
			if !slices.Contains(names, pack.Type) {
				t.Fatalf("%s unexpectedly modifies %s", template, pack.Type)
			}
		}
		publish(request)
		assertCustomMasterSources(t, original, rows)
		unchanged, err := masterdata.PrepareCustom(ctx, template)
		if err != nil || len(unchanged.Packs) != 0 {
			t.Fatalf("%s is not idempotent: %v", template, err)
		}
		if err := masterdata.RemoveStaging(unchanged.ID); err != nil {
			t.Fatal(err)
		}
	}
	assertCustomMasterRows(t, "HomeAction", customMasterExpectedHome(rows))
	for name, expected := range customMasterExpectedPhoto(rows) {
		assertCustomMasterRows(t, name, expected)
	}
	homeReset, err := masterdata.PrepareReset(ctx, masterdata.ResetOptions{Types: []string{"HomeAction"}})
	if err != nil {
		t.Fatal(err)
	}
	publish(homeReset)
	assertCustomMasterRows(t, "HomeAction", rows["HomeAction"])
	for name, expected := range customMasterExpectedPhoto(rows) {
		assertCustomMasterRows(t, name, expected)
	}
	names, _ := masterdata.CustomTypes("photo")
	photoReset, err := masterdata.PrepareReset(ctx, masterdata.ResetOptions{Types: names})
	if err != nil || len(photoReset.Packs) != 4 {
		t.Fatalf("targeted photo reset failed: %v", err)
	}
	publish(photoReset)
	for name, expected := range rows {
		assertCustomMasterRows(t, name, expected)
	}
	if !proto.Equal(masterPublicationActive(t), original) {
		t.Fatal("reset did not restore the complete original manifest and version")
	}
	assertCustomMasterSources(t, original, rows)
}

func TestCustomMasterValidationScenario(t *testing.T) {
	original, rows := setupCustomMaster(t)
	ctx := context.Background()
	if _, err := masterdata.PrepareCustom(ctx, "unknown-template"); err == nil {
		t.Fatal("unknown template was accepted")
	}
	for _, options := range []masterdata.ResetOptions{
		{Type: "HomeAction", Types: []string{"HomeAction"}},
		{All: true, Types: []string{"HomeAction"}},
		{Types: []string{"HomeAction", "HomeAction"}},
		{Types: []string{""}},
	} {
		if _, err := masterdata.PrepareReset(ctx, options); err == nil {
			t.Fatal("invalid multi-type reset selection was accepted")
		}
	}
	valid, err := proto.Marshal(rows["LoveHomeAction"][1])
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := proto.Marshal(rows["LoveHomeAction"][0])
	if err != nil {
		t.Fatal(err)
	}
	unknown := append([]byte(nil), valid...)
	unknown = protowire.AppendTag(unknown, 9999, protowire.VarintType)
	unknown = protowire.AppendVarint(unknown, 1)
	replaceSource := func(data []byte) {
		t.Helper()
		if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
			_, err := mongo.SessionFromContext(ctx).Client().Database("master").Collection("LoveHomeAction").UpdateOne(ctx,
				bson.M{"Id": "love-two"}, bson.M{"$set": bson.M{"data": data}})
			return nil, err
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, data := range [][]byte{{0xff}, unknown, duplicate} {
		replaceSource(data)
		if _, err := masterdata.PrepareCustom(ctx, "home-action"); err == nil {
			t.Fatal("malformed, unknown-field or duplicate-ID source was accepted")
		}
		assertCustomMasterRows(t, "HomeAction", rows["HomeAction"])
		if !proto.Equal(masterPublicationActive(t), original) || masterOfflineJobCount(t) != 0 {
			t.Fatal("failed customization modified the active manifest or created a job")
		}
	}
	replaceSource(valid)
	// An incomplete target set must never customize the remaining three packs.
	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		_, err := mongo.SessionFromContext(ctx).Client().Database("solis").Collection(sname.MasterTagPack).DeleteOne(ctx, bson.M{"type": "PhotoQuestStage"})
		return nil, err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := masterdata.PrepareCustom(ctx, "photo"); err == nil {
		t.Fatal("missing target master was accepted")
	}
	for _, pack := range original.MasterTagPacks {
		if pack.Type == "PhotoQuestStage" {
			if err := opsolis.UpsertMasterTagPack(ctx, pack); err != nil {
				t.Fatal(err)
			}
		}
	}
	stale, err := masterdata.PrepareCustom(ctx, "home-action")
	if err != nil {
		t.Fatal(err)
	}
	photo, err := masterdata.PrepareCustom(ctx, "photo")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := masterdata.AcquireOwnership()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if _, err := owner.Publish(ctx, *photo); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Publish(ctx, *stale); err == nil {
		t.Fatal("stale customization was accepted")
	}
	assertCustomMasterRows(t, "HomeAction", rows["HomeAction"])
	for name, expected := range customMasterExpectedPhoto(rows) {
		assertCustomMasterRows(t, name, expected)
	}
	assertCustomMasterSources(t, original, rows)
}

func TestCustomMasterCLIScenario(t *testing.T) {
	binary := os.Getenv("MAKINO_TEST_CLI_BINARY")
	if binary == "" || !filepath.IsAbs(binary) {
		t.Skip("requires absolute MAKINO_TEST_CLI_BINARY")
	}
	original, rows := setupCustomMaster(t)
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	command := func(success bool, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, "SOLIS_ADMIN_AUTH=") {
				cmd.Env = append(cmd.Env, value)
			}
		}
		cmd.Env = append(cmd.Env, "SOLIS_ADMIN_AUTH=")
		isolateTestCommand(t, cmd, repo)
		output, err := os.CreateTemp(t.TempDir(), "custom-output-*.log")
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		cmd.Stdout, cmd.Stderr = output, output
		runErr := cmd.Run()
		if _, err := output.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		text, err := io.ReadAll(io.LimitReader(output, 65536))
		if err != nil {
			t.Fatal(err)
		}
		if success != (runErr == nil) {
			t.Fatalf("unexpected custom command outcome: %v\n%s", runErr, text)
		}
		return string(text)
	}
	if text := command(true, "custom", "--help"); !strings.Contains(text, "home-action") || !strings.Contains(text, "photo") {
		t.Fatal("custom help omitted available templates")
	}
	command(false, "custom", "unknown")
	command(false, "custom", "photo", "--unexpected")
	command(false, "custom", "photo", "unexpected")
	command(true, "custom", "home-action", "--dry-run")
	if !proto.Equal(masterPublicationActive(t), original) || masterOfflineJobCount(t) != 0 {
		t.Fatal("custom dry-run changed active data or created a publication job")
	}
	command(true, "custom", "home-action")
	assertCustomMasterRows(t, "HomeAction", customMasterExpectedHome(rows))
	jobs := masterOfflineJobCount(t)
	command(true, "custom", "home-action")
	if masterOfflineJobCount(t) != jobs {
		t.Fatal("idempotent custom command created another publication job")
	}
	command(true, "custom", "photo")
	command(true, "custom", "home-action", "--reset")
	assertCustomMasterRows(t, "HomeAction", rows["HomeAction"])
	for name, expected := range customMasterExpectedPhoto(rows) {
		assertCustomMasterRows(t, name, expected)
	}
	custom := masterPublicationActive(t)
	command(true, "custom", "photo", "--reset", "--dry-run")
	if !proto.Equal(masterPublicationActive(t), custom) {
		t.Fatal("custom reset dry-run published changes")
	}
	command(true, "custom", "photo", "--reset")
	if !proto.Equal(masterPublicationActive(t), original) {
		t.Fatal("CLI reset did not restore original manifests")
	}
	for name, expected := range rows {
		assertCustomMasterRows(t, name, expected)
	}
	assertCustomMasterSources(t, original, rows)
}
