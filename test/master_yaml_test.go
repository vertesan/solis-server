package test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/utils/config"
	mastercrypto "github.com/vertesan/solis-server/internal/utils/crypto"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func masterYAMLHeader(name string) string {
	return "formatVersion: 1\ntype: " + name + "\nbaseVersion: test-version\nbaseFileName: \"" + strings.Repeat("a", 64) + "\"\n---\n"
}

func TestMasterYAMLRoundTripScenario(t *testing.T) {
	// Local serialization does not require a game account or mutate MongoDB.
	input := masterYAMLHeader("Card") + `data:
  id: card-example
  name: Example
  type: CardType_Appeal
  initialRarity: 5
  releaseDate: 9223372036854775807
  rewardHairIds: [hair-a, hair-b]
  stories:
    - storyId: story-a
---
data:
  id: second-card
  releaseDate: -9223372036854775808
`
	reader, err := masterdata.NewYAMLReader(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	first, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	wire, err := proto.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var card pmaster.Card
	if err := proto.Unmarshal(wire, &card); err != nil {
		t.Fatal(err)
	}
	if card.ReleaseDate != math.MaxInt64 || card.Type != penum.CardType_CardType_Appeal || len(card.RewardHairIds) != 2 || card.GetStories()[0].GetStoryId() != "story-a" {
		t.Fatalf("protobuf values were not preserved: %v", &card)
	}
	second, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	field := second.ProtoReflect().Descriptor().Fields().ByName("releaseDate")
	if second.ProtoReflect().Get(field).Int() != math.MinInt64 {
		t.Fatal("signed lower integer boundary was rounded")
	}
	if _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected end of stream, got %v", err)
	}
	for name, record := range map[string]string{
		"unknown":               "data: {id: card, madeUp: true}",
		"nested unknown":        "data: {stories: [{madeUp: true}]}",
		"invalid enum":          "data: {type: CardType_NotReal}",
		"overflow":              "data: {releaseDate: 9223372036854775808}",
		"fractional integer":    "data: {initialRarity: 1.25}",
		"duplicate field":       "data: {id: card, id: duplicate}",
		"duplicate wrapper":     "data: {}\ndata: {}",
		"extra editable column": "id: card\ndata: {}",
		"alias":                 "data: {id: &id card, name: *id}",
		"null":                  "data: null",
		"missing":               "{}",
		"sequence":              "data: []",
	} {
		t.Run(name, func(t *testing.T) {
			reader, err := masterdata.NewYAMLReader(strings.NewReader(masterYAMLHeader("Card") + record))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reader.Next(); err == nil {
				t.Fatal("invalid document was accepted")
			}
		})
	}
	unknown := protowire.AppendTag(nil, 5000, protowire.VarintType)
	unknown = protowire.AppendVarint(unknown, 1)
	for _, message := range []proto.Message{&pmaster.Card{}, &pmaster.Card{Stories: []*pmaster.CardStory{{StoryId: "story"}}}} {
		if card := message.(*pmaster.Card); len(card.Stories) != 0 {
			card.Stories[0].ProtoReflect().SetUnknown(unknown)
		} else {
			card.ProtoReflect().SetUnknown(unknown)
		}
		data, err := proto.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := masterdata.DecodeRecord("Card", data); err == nil {
			t.Fatal("unknown wire fields would silently disappear during export")
		}
	}
	composite := &mastercrypto.MasterDbSchema{Table: "LoveLevel", Columns: []mastercrypto.MasterDbColumn{{Name: "loveId", Type: "TEXT", PrimaryKey: 1}, {Name: "level", Type: "INTEGER", PrimaryKey: 2}, {Name: "data", Type: "BLOB"}}}
	row, err := masterdata.ProjectRecord(composite, &pmaster.LoveLevel{LoveId: "love-example", Level: 7, RequiredExp: math.MaxInt64})
	if err != nil {
		t.Fatal(err)
	}
	if row["loveId"] != "love-example" || row["level"] != int64(7) {
		t.Fatalf("composite indexed values not derived: %v", row)
	}
	composite.Columns = append(composite.Columns, mastercrypto.MasterDbColumn{Name: "notInProto", Type: "TEXT"})
	if _, err := masterdata.ProjectRecord(composite, &pmaster.LoveLevel{}); err == nil {
		t.Fatal("unsupported schema mapping accepted")
	}
}

func TestMasterYAMLExportScenario(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	path := filepath.Join(directory, "Card.yaml")
	options := masterdata.ExportOptions{Type: "Card", File: path}
	if err := masterdata.Export(ctx, options); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(before, []byte("cryptoKey:")) || bytes.Contains(before, []byte("_id:")) {
		t.Fatal("export exposes internal fields")
	}
	if err := masterdata.Export(ctx, options); err == nil {
		t.Fatal("overwriting export did not require --force")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("rejected export changed existing file")
	}
	options.Force = true
	if err := masterdata.Export(ctx, options); err != nil {
		t.Fatal(err)
	}
	reader, err := masterdata.NewYAMLReader(bytes.NewReader(before))
	if err != nil {
		t.Fatal(err)
	}
	var count int
	for {
		if _, err := reader.Next(); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count == 0 {
		t.Fatal("Card export unexpectedly empty")
	}
	profile, err := opsolis.FindServerProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reader.Header.BaseVersion != profile.MasterVersion {
		t.Fatal("export has incorrect active version")
	}
}

func TestMasterYAMLIncrementalScenario(t *testing.T) {
	var input strings.Builder
	input.WriteString(masterYAMLHeader("LoveLevel"))
	for index := 0; index < 2000; index++ {
		if index > 0 {
			input.WriteString("---\n")
		}
		fmt.Fprintf(&input, "data: {loveId: love-example, level: %d, requiredExp: 9007199254740993}\n", index)
	}
	reader, err := masterdata.NewYAMLReader(strings.NewReader(input.String()))
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2000; index++ {
		message, err := reader.Next()
		if err != nil {
			t.Fatal(err)
		}
		field := message.ProtoReflect().Descriptor().Fields().ByName("requiredExp")
		if message.ProtoReflect().Get(field).Int() != 9007199254740993 {
			t.Fatal("integer precision lost")
		}
	}
	if _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}

func TestMasterYAMLAllPacksScenario(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	if err := masterdata.Export(ctx, masterdata.ExportOptions{All: true, Directory: directory}); err != nil {
		t.Fatal(err)
	}
	packs, err := opsolis.GetAllMasterTagPack(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, pack := range packs {
		file, err := os.Open(filepath.Join(directory, pack.Type+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		reader, err := masterdata.NewYAMLReader(file)
		if err != nil {
			file.Close()
			t.Fatalf("%s: %v", pack.Type, err)
		}
		if reader.Header.Type != pack.Type || reader.Header.BaseFileName != pack.FileName {
			file.Close()
			t.Fatalf("%s header mismatch", pack.Type)
		}
		for {
			if _, err := reader.Next(); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				file.Close()
				t.Fatalf("%s record: %v", pack.Type, err)
			}
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(packs) {
		t.Fatalf("all-pack export returned %d files for %d packs", len(entries), len(packs))
	}
}

// Regenerating SQL columns from decoded protobufs exercises affinities that a
// raw SQL-to-SQL copy cannot reveal, such as numeric-looking STRING identifiers.
func TestMasterProjectionFreshAllPacksScenario(t *testing.T) {
	ctx := context.Background()
	packs, err := opsolis.GetAllMasterTagPack(ctx)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	for _, pack := range packs {
		if os.Getenv("SOLIS_TEST_MASTER_ALL_PACKS") != "1" && pack.Type != "GachaExchange" {
			continue
		}
		t.Run(pack.Type, func(t *testing.T) {
			source := filepath.Join(config.GetDBDir(), pack.FileName)
			schema, err := mastercrypto.InspectMasterDb(ctx, source, pack.CryptoKey, pack.Type)
			if err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(directory, pack.Type)
			if err := mastercrypto.BuildMasterDb(ctx, source, destination, pack.CryptoKey, pack.Type, func(yield func(map[string]any) error) error {
				return mastercrypto.IterateMasterDb(ctx, source, pack.CryptoKey, pack.Type, func(row map[string]any) error {
					message, err := masterdata.DecodeRecord(pack.Type, row["data"].([]byte))
					if err != nil {
						return err
					}
					projected, err := masterdata.ProjectRecord(schema, message)
					if err != nil {
						return err
					}
					for name, value := range projected {
						if name != "data" && !reflect.DeepEqual(value, row[name]) {
							return fmt.Errorf("column %s projection (%T %v) differs from source (%T %v)", name, value, value, row[name], row[name])
						}
					}
					return yield(projected)
				})
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := masterdata.ValidatePack(ctx, destination, pack.CryptoKey, pack.Type); err != nil {
				t.Fatal(err)
			}
		})
	}
}
