package test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.yaml.in/yaml/v3"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// Owns its child server and uses a temporary resource tree, so immediate old
// pack deletion is exercised without touching the administrator's actual files.
func TestMasterPublicationLifecycleScenario(t *testing.T) {
	serverBinary, cliBinary := os.Getenv("MAKINO_TEST_SERVER_BINARY"), os.Getenv("MAKINO_TEST_CLI_BINARY")
	if serverBinary == "" || cliBinary == "" || os.Getenv("MAKINO_TEST_MASTER_LIFECYCLE") != "1" {
		t.Skip("requires master lifecycle opt-in and built server/CLI binaries")
	}
	if testConfig.Env != Dev {
		t.Fatal("master publication test requires development configuration")
	}
	parsedHTTP, err := url.Parse(testConfig.HttpUrl)
	if err != nil {
		t.Fatal(err)
	}
	addresses := []string{testConfig.GrpcFqdn, parsedHTTP.Host}
	for _, address := range addresses {
		if conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond); err == nil {
			conn.Close()
			t.Fatalf("%s is occupied; existing server was not touched", address)
		}
	}
	ctx := context.Background()
	profile, err := opsolis.FindServerProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	packs, err := opsolis.GetAllMasterTagPack(ctx)
	if err != nil {
		t.Fatal(err)
	}
	originalResource := config.GetResourceDir()
	backupMakinoCatalogCollections(t, sname.ServerProfile, sname.MasterTagPack, sname.MasterTagPackOrig, sname.MasterImportJob,
		sname.NoticeCatalog, sname.ShopCatalog, sname.ExchangeCatalog, sname.GachaCatalog, sname.GachaRules)
	backupMasterOfficialCollections(t, "Card", "_version")
	resource := t.TempDir()
	for _, dir := range []string{"db", "manifest"} {
		if err := os.Mkdir(filepath.Join(resource, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range packs {
		copyMasterTestFile(t, filepath.Join(originalResource, "db", p.FileName), filepath.Join(resource, "db", p.FileName))
	}
	entries, err := os.ReadDir(filepath.Join(originalResource, "manifest"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			copyMasterTestFile(t, filepath.Join(originalResource, "manifest", entry.Name()), filepath.Join(resource, "manifest", entry.Name()))
		}
	}
	t.Setenv("SOLIS_RESOURCE_DIR", resource)
	// Fixtures establish a baseline; the separate official-update scenario
	// verifies the explicitly requested manual first-backup trigger.
	if err := opsolis.ReplaceOfficialMasterTag(ctx, &ptransaction.MasterTag{Version: profile.MasterVersion, MasterTagPacks: packs}); err != nil {
		t.Fatal(err)
	}
	repoDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	var process *makinoTestServerProcess
	t.Cleanup(func() {
		if process != nil {
			process.stop(t)
		}
		testConfig.HttpClient.CloseIdleConnections()
	})
	process = startMakinoTestServer(t, serverBinary, repoDir, filepath.Join(resource, "server.log"), addresses)
	setupTestUser(t)
	command := func(args ...string) []byte {
		t.Helper()
		commandCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(commandCtx, cliBinary, args...)
		cmd.Dir = repoDir
		var output, stderr bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("master CLI failed: %v\n%s", err, stderr.String())
		}
		return output.Bytes()
	}
	cardFile := filepath.Join(resource, "Card.yaml")
	command("master", "export", "--type", "Card", "--file", cardFile)
	// A full unchanged export/import also exercises large and all-pack parsing
	// without any publication or gameplay pause.
	allDir := filepath.Join(resource, "all")
	command("master", "export", "--all", "--dir", allDir)
	if result := command("master", "import", "--all", "--dir", allDir, "--dry-run"); !bytes.Contains(result, []byte("0 changed pack(s)")) {
		t.Fatalf("unchanged all-pack import was not a no-op: %s", result)
	}
	cardID := editMasterCardYAML(t, cardFile, "Makino publication test")
	if result := command("master", "import", "--file", cardFile, "--dry-run"); !bytes.Contains(result, []byte("1 changed pack(s)")) {
		t.Fatalf("modified Card was not detected: %s", result)
	}
	oldCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	var accepted masterdata.ImportJob
	if err := json.Unmarshal(command("master", "import", "--file", cardFile), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.ID == "" {
		t.Fatal("import did not return job ID")
	}
	var completed masterdata.ImportJob
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		if err := json.Unmarshal(command("master", "status", "--id", accepted.ID), &completed); err != nil {
			t.Fatal(err)
		}
		if completed.State == "complete" {
			break
		}
		if completed.State == "failed" || completed.State == "cleanup_pending" {
			t.Fatalf("publication %s: %s", completed.State, completed.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if completed.State != "complete" || !completed.Committed {
		t.Fatalf("publication did not complete: %s", completed.State)
	}
	staleCtx := metadata.AppendToOutgoingContext(oldCtx, "x-app-request-id", "9223372036854770000")
	staleResponse := callGRPCWire(t, staleCtx, papi.Home_Enter_FullMethodName, &papi.HomeEnterRequest{})
	assertGRPCWireAction(t, staleResponse, "title", "1002", "マスターデータが更新されました")
	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		count, err := mongo.SessionFromContext(ctx).Client().Database("user").Collection(uname.UserAppRequest).CountDocuments(ctx, bson.M{"SUID": testConfig.TestUserSuid, "requestId": "9223372036854770000"})
		if err == nil && count != 0 {
			return nil, fmt.Errorf("outdated request reserved an idempotency key")
		}
		return nil, err
	}); err != nil {
		t.Fatal(err)
	}
	tag := profileCall(t, papi.NewMasterClient(testConfig.GrpcConn).Get, &papi.Empty{}).GetMasterTag()
	if tag.GetVersion() != completed.Version {
		t.Fatal("Master/Get snapshot version differs from committed job")
	}
	var oldCard, newCard *ptransaction.MasterTagPack
	for _, p := range packs {
		if p.Type == "Card" {
			oldCard = p
		}
	}
	for _, p := range tag.MasterTagPacks {
		if p.Type == "Card" {
			newCard = p
		}
	}
	if oldCard == nil || newCard == nil || oldCard.FileName == newCard.FileName {
		t.Fatal("Card manifest did not change")
	}
	if _, err := os.Stat(filepath.Join(config.GetDBDir(), oldCard.FileName)); err != nil {
		t.Fatalf("superseded original pack must remain available for reset: %v", err)
	}
	for _, p := range packs {
		if p.Type != "Card" {
			if _, err := os.Stat(filepath.Join(config.GetDBDir(), p.FileName)); err != nil {
				t.Fatalf("unselected pack deleted: %s", p.Type)
			}
		}
	}
	card, err := opmaster.FindCard(ctx, cardID)
	if err != nil || card.GetName() != "Makino publication test" {
		t.Fatalf("customized Mongo protobuf not published: %v", err)
	}
	originals, err := opsolis.GetAllOriginalMasterTagPack(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range originals {
		if p.Type == "Card" && !proto.Equal(p, oldCard) {
			t.Fatal("custom publication modified original manifest")
		}
	}
	downloadURL, err := url.Parse(newCard.DownloadUrl)
	if err != nil {
		t.Fatal(err)
	}
	downloadURL.Scheme = parsedHTTP.Scheme
	downloadURL.Host = parsedHTTP.Host
	response, err := http.Get(downloadURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	h := sha256.New()
	size, err := io.Copy(h, response.Body)
	if err != nil || response.StatusCode != 200 || size != int64(newCard.FileSize) || hex.EncodeToString(h.Sum(nil)) != newCard.FileName {
		t.Fatalf("download differs from manifest: HTTP %d bytes %d err %v", response.StatusCode, size, err)
	}
	process.stop(t)
	testConfig.HttpClient.CloseIdleConnections()
	process = startMakinoTestServer(t, serverBinary, repoDir, filepath.Join(resource, "restart.log"), addresses)
	after := profileCall(t, papi.NewMasterClient(testConfig.GrpcConn).Get, &papi.Empty{}).GetMasterTag()
	if !proto.Equal(tag, after) {
		t.Fatal("restart replaced custom manifest")
	}
	card, err = opmaster.FindCard(ctx, cardID)
	if err != nil || card.GetName() != "Makino publication test" {
		t.Fatal("restart replaced customized master rows")
	}
}

func copyMasterTestFile(t *testing.T, source, destination string) {
	t.Helper()
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		t.Fatal(copyErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}

func editMasterCardYAML(t *testing.T, filename, name string) string {
	t.Helper()
	input, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(input))
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	selected := ""
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if selected == "" && len(document.Content) > 0 {
			root := document.Content[0]
			for i := 0; i+1 < len(root.Content); i += 2 {
				if root.Content[i].Value != "data" {
					continue
				}
				data := root.Content[i+1]
				for j := 0; j+1 < len(data.Content); j += 2 {
					if data.Content[j].Value == "id" {
						selected = data.Content[j+1].Value
					}
				}
				found := false
				for j := 0; j+1 < len(data.Content); j += 2 {
					if data.Content[j].Value == "name" {
						data.Content[j+1].Value = name
						found = true
					}
				}
				if !found {
					data.Content = append(data.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "name"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name})
				}
			}
		}
		if err := encoder.Encode(&document); err != nil {
			t.Fatal(err)
		}
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(selected) == "" {
		t.Fatal("no editable Card row found")
	}
	if err := os.WriteFile(filename, output.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return selected
}
