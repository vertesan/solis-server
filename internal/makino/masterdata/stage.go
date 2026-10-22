package masterdata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	mastercrypto "github.com/vertesan/solis-server/internal/utils/crypto"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ExportOptions struct {
	Type, File, Directory string
	All, Force            bool
	Output                io.Writer
}
type ImportOptions struct {
	Files           []string
	All, AllowEmpty bool
}

func activeManifest(ctx context.Context) (string, map[string]*ptransaction.MasterTagPack, error) {
	profile, err := opsolis.FindServerProfile(ctx)
	if err != nil {
		return "", nil, err
	}
	packs, err := opsolis.GetAllMasterTagPack(ctx)
	if err != nil {
		return "", nil, err
	}
	result := make(map[string]*ptransaction.MasterTagPack, len(packs))
	for _, pack := range packs {
		if pack == nil || !encryptedFileName.MatchString(pack.FileName) {
			return "", nil, errors.New("active manifest contains invalid encrypted filenames")
		}
		if _, err := masterDescriptor(pack.Type); err != nil {
			return "", nil, err
		}
		if result[pack.Type] != nil {
			return "", nil, fmt.Errorf("duplicate active master type %s", pack.Type)
		}
		result[pack.Type] = pack
	}
	if profile.MasterVersion == "" || len(result) == 0 {
		return "", nil, errors.New("active master manifest is empty")
	}
	return profile.MasterVersion, result, nil
}

func unchangedVersion(ctx context.Context, expected string) error {
	profile, err := opsolis.FindServerProfile(ctx)
	if err != nil {
		return err
	}
	if profile.MasterVersion != expected {
		return errors.New("master version changed during processing; export again and retry")
	}
	return nil
}

func Export(ctx context.Context, options ExportOptions) (retErr error) {
	if options.All == (options.Type != "") {
		return errors.New("export requires exactly one of --type or --all")
	}
	if options.All && (options.Directory == "" || options.Output != nil) {
		return errors.New("--all exports require --dir; bulk export to stdout is not supported")
	}
	if options.Output != nil {
		if options.File != "" || options.Directory != "" {
			return errors.New("export output cannot be combined with --file or --dir")
		}
	} else if (options.All && (options.Directory == "" || options.File != "")) || (!options.All && (options.File == "" || options.Directory != "")) {
		return errors.New("file export requires --type and --file, or --all and --dir")
	}
	version, packs, err := activeManifest(ctx)
	if err != nil {
		return err
	}
	types := []string{options.Type}
	if options.All {
		types = types[:0]
		for name := range packs {
			types = append(types, name)
		}
		sort.Strings(types)
	}
	for _, name := range types {
		if packs[name] == nil {
			return fmt.Errorf("master type %q is not active", name)
		}
	}
	if options.Output != nil {
		return exportStream(ctx, version, packs[options.Type], options.Output)
	}
	targets := make(map[string]string, len(types))
	for _, name := range types {
		target := options.File
		if options.All {
			target = filepath.Join(options.Directory, name+".yaml")
		}
		if info, err := os.Lstat(target); err == nil {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("export target %s is not a regular file", target)
			}
			if !options.Force {
				return fmt.Errorf("export target %s already exists; use --force", target)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		targets[name] = target
	}
	temporary := make(map[string]string, len(types))
	defer func() {
		for _, path := range temporary {
			_ = os.Remove(path)
		}
	}()
	for _, name := range types {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := targets[name]
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		file, err := os.CreateTemp(filepath.Dir(target), ".master-export-*.yaml")
		if err != nil {
			return err
		}
		temporary[name] = file.Name()
		encoder := yaml.NewEncoder(file)
		encoder.SetIndent(2)
		err = writeExportTable(ctx, encoder, version, packs[name])
		err = errors.Join(err, encoder.Close(), file.Sync(), file.Close())
		if err != nil {
			return fmt.Errorf("export %s: %w", name, err)
		}
	}
	if err := unchangedVersion(ctx, version); err != nil {
		return err
	}
	for _, name := range types {
		if options.Force {
			err = os.Rename(temporary[name], targets[name])
		} else {
			err = os.Link(temporary[name], targets[name])
		}
		if err != nil {
			return fmt.Errorf("publish exported %s: %w", name, err)
		}
	}
	return nil
}

func writeExportTable(ctx context.Context, encoder *yaml.Encoder, version string, pack *ptransaction.MasterTagPack) error {
	if err := encoder.Encode(YAMLHeader{FormatVersion: 1, Type: pack.Type, BaseVersion: version, BaseFileName: pack.FileName}); err != nil {
		return err
	}
	return opmaster.IterateMasterPayloads(ctx, pack.Type, func(data []byte) error {
		message, err := DecodeRecord(pack.Type, data)
		if err != nil {
			return err
		}
		return writeRecord(encoder, message)
	})
}

// Spool until validation and the version fence succeed, just as file exports
// do. This keeps memory bounded and avoids sending invalid partial YAML to a
// pipeline when a record is unsupported or a publication happens mid-export.
func exportStream(ctx context.Context, version string, pack *ptransaction.MasterTagPack, output io.Writer) (retErr error) {
	file, err := os.CreateTemp("", ".solis-master-export-*.yaml")
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, file.Close(), os.Remove(file.Name())) }()
	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(2)
	if err = ctx.Err(); err == nil {
		err = writeExportTable(ctx, encoder, version, pack)
	}
	if err = errors.Join(err, encoder.Close()); err != nil {
		return err
	}
	if err := unchangedVersion(ctx, version); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err = io.Copy(output, file)
	return err
}

// PrepareImport validates and encrypts the whole batch before asking the live
// server to pause gameplay. Failure removes only this invocation's staging files.
func PrepareImport(ctx context.Context, options ImportOptions) (_ *ImportRequest, retErr error) {
	if len(options.Files) == 0 {
		return nil, errors.New("no master YAML files supplied")
	}
	version, packs, err := activeManifest(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := opsolis.FindServerProfile(ctx)
	if err != nil {
		return nil, err
	}
	if profile.MasterVersionOrig == "" {
		return nil, errors.New("original master baseline is missing; trigger a normal official update before importing")
	}
	if err := validateOriginalBaseline(ctx, packs); err != nil {
		return nil, err
	}
	request := &ImportRequest{ID: uuid.NewString(), ExpectedVersion: version, AllowEmpty: options.AllowEmpty}
	directory, err := StagingDirectory(config.GetDBDir(), request.ID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(directory), 0700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil || len(request.Packs) == 0 {
			_ = RemoveStaging(request.ID)
		}
	}()
	seen := map[string]bool{}
	for _, source := range options.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := regularFile(source); err != nil {
			return nil, err
		}
		input, err := os.Open(source)
		if err != nil {
			return nil, err
		}
		reader, err := NewYAMLReader(input)
		if err != nil {
			input.Close()
			return nil, fmt.Errorf("%s: %w", source, err)
		}
		header := reader.Header
		pack := packs[header.Type]
		if pack == nil || header.BaseVersion != version || header.BaseFileName != pack.FileName {
			input.Close()
			return nil, fmt.Errorf("%s has a stale or inactive master header; export again", source)
		}
		if seen[header.Type] {
			input.Close()
			return nil, fmt.Errorf("duplicate input master type %s", header.Type)
		}
		seen[header.Type] = true
		staged, err := stageTable(ctx, directory, pack, reader, options.AllowEmpty)
		err = errors.Join(err, input.Close())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", source, err)
		}
		if staged != nil {
			request.Packs = append(request.Packs, *staged)
		}
	}
	if options.All && len(seen) != len(packs) {
		return nil, fmt.Errorf("--all requires every active master type: supplied %d of %d", len(seen), len(packs))
	}
	if err := unchangedVersion(ctx, version); err != nil {
		return nil, err
	}
	return request, nil
}

func stageTable(ctx context.Context, directory string, pack *ptransaction.MasterTagPack, reader *YAMLReader, allowEmpty bool) (*StagedPack, error) {
	active := filepath.Join(config.GetDBDir(), pack.FileName)
	if err := regularFile(active); err != nil {
		return nil, err
	}
	schema, err := mastercrypto.InspectMasterDb(ctx, active, pack.CryptoKey, pack.Type)
	if err != nil {
		return nil, err
	}
	if _, err := schemaFields(schema, reader.descriptor); err != nil {
		return nil, err
	}
	var previous, next semanticDigest
	if err := mastercrypto.IterateMasterDb(ctx, active, pack.CryptoKey, pack.Type, func(row map[string]any) error {
		data, ok := row["data"].([]byte)
		if !ok {
			return errors.New("master data column is not binary")
		}
		message, err := DecodeRecord(pack.Type, data)
		if err != nil {
			return err
		}
		return previous.add(message)
	}); err != nil {
		return nil, err
	}
	// The incomplete filename cannot be submitted to the publication API.
	temporary := filepath.Join(directory, pack.Type+".building")
	if err := mastercrypto.BuildMasterDb(ctx, active, temporary, pack.CryptoKey, pack.Type, func(insert func(map[string]any) error) error {
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			message, err := reader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			row, err := ProjectRecord(schema, message)
			if err != nil {
				return err
			}
			if err := insert(row); err != nil {
				return fmt.Errorf("invalid or duplicate master key: %w", err)
			}
			if err := next.add(message); err != nil {
				return err
			}
		}
		if next.Count == 0 && previous.Count != 0 && !allowEmpty {
			return errors.New("emptying a nonempty master collection requires --allow-empty")
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if next == previous {
		return nil, os.Remove(temporary)
	}
	count, err := ValidatePack(ctx, temporary, pack.CryptoKey, pack.Type)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(temporary)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(hash, file)
	err = errors.Join(copyErr, file.Close())
	if err != nil {
		return nil, err
	}
	if size > math.MaxInt32 {
		return nil, errors.New("encrypted master file exceeds manifest fileSize capacity")
	}
	name := hex.EncodeToString(hash.Sum(nil))
	if err := os.Rename(temporary, filepath.Join(directory, name)); err != nil {
		return nil, err
	}
	return &StagedPack{Type: pack.Type, BaseFileName: pack.FileName, FileName: name, FileSize: size, RowCount: count}, nil
}

func schemaFields(schema *mastercrypto.MasterDbSchema, descriptor protoreflect.MessageDescriptor) (map[string]protoreflect.FieldDescriptor, error) {
	fields := map[string]protoreflect.FieldDescriptor{}
	dataFound := false
	for _, column := range schema.Columns {
		if column.Name == "data" {
			dataFound = true
			continue
		}
		if column.Hidden != 0 {
			return nil, fmt.Errorf("generated master column %q is unsupported", column.Name)
		}
		var match protoreflect.FieldDescriptor
		for i := 0; i < descriptor.Fields().Len(); i++ {
			field := descriptor.Fields().Get(i)
			if strings.EqualFold(column.Name, string(field.Name())) || strings.EqualFold(column.Name, field.JSONName()) {
				if match != nil {
					return nil, fmt.Errorf("ambiguous master column %s", column.Name)
				}
				match = field
			}
		}
		if match == nil || match.IsList() || match.IsMap() || match.Kind() == protoreflect.MessageKind || match.Kind() == protoreflect.GroupKind {
			return nil, fmt.Errorf("unsupported master column mapping %s.%s", schema.Table, column.Name)
		}
		fields[column.Name] = match
	}
	if !dataFound {
		return nil, errors.New("master table has no data column")
	}
	return fields, nil
}

// ProjectRecord regenerates every indexed SQL/Mongo column from the protobuf.
// SQLite's original primary/unique constraints also cover composite keys.
func ProjectRecord(schema *mastercrypto.MasterDbSchema, message proto.Message) (map[string]any, error) {
	fields, err := schemaFields(schema, message.ProtoReflect().Descriptor())
	if err != nil {
		return nil, err
	}
	data, err := (proto.MarshalOptions{Deterministic: true}).Marshal(message)
	if err != nil {
		return nil, err
	}
	row := map[string]any{"data": data}
	for name, field := range fields {
		value := message.ProtoReflect().Get(field)
		switch field.Kind() {
		case protoreflect.StringKind:
			row[name] = value.String()
		case protoreflect.BytesKind:
			row[name] = value.Bytes()
		case protoreflect.BoolKind:
			if value.Bool() {
				row[name] = int64(1)
			} else {
				row[name] = int64(0)
			}
		case protoreflect.EnumKind:
			row[name] = int64(value.Enum())
		case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Sint32Kind, protoreflect.Sint64Kind, protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
			row[name] = value.Int()
		case protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.Fixed32Kind, protoreflect.Fixed64Kind:
			if value.Uint() > math.MaxInt64 {
				return nil, fmt.Errorf("indexed field %s exceeds SQLite signed integer range", name)
			}
			row[name] = int64(value.Uint())
		case protoreflect.FloatKind, protoreflect.DoubleKind:
			row[name] = value.Float()
		default:
			return nil, fmt.Errorf("unsupported indexed protobuf field %s", name)
		}
	}
	// SQLite's declared STRING type has NUMERIC (not TEXT) affinity. For
	// example, an official GachaExchange id "-001" is indexed as integer -1,
	// while its protobuf payload must retain the exact original string.
	for _, column := range schema.Columns {
		value, err := mastercrypto.NormalizeMasterValue(column, row[column.Name])
		if err != nil {
			return nil, err
		}
		row[column.Name] = value
	}
	return row, nil
}

func ValidatePack(ctx context.Context, path, key, masterType string) (int64, error) {
	schema, err := mastercrypto.InspectMasterDb(ctx, path, key, masterType)
	if err != nil {
		return 0, err
	}
	descriptor, err := masterDescriptor(masterType)
	if err != nil {
		return 0, err
	}
	if _, err := schemaFields(schema, descriptor); err != nil {
		return 0, err
	}
	var count int64
	err = mastercrypto.IterateMasterDb(ctx, path, key, masterType, func(row map[string]any) error {
		data, ok := row["data"].([]byte)
		if !ok {
			return errors.New("master data column is not binary")
		}
		message, err := DecodeRecord(masterType, data)
		if err != nil {
			return err
		}
		derived, err := ProjectRecord(schema, message)
		if err != nil {
			return err
		}
		for name, value := range derived {
			if name != "data" && !reflect.DeepEqual(value, row[name]) {
				return fmt.Errorf("%s.%s disagrees with its protobuf payload", masterType, name)
			}
		}
		count++
		return nil
	})
	return count, err
}

type semanticDigest struct {
	Sum   [32]byte
	Count int64
}

func (digest *semanticDigest) add(message proto.Message) error {
	data, err := (proto.MarshalOptions{Deterministic: true}).Marshal(message)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	carry := uint16(0)
	for index := 31; index >= 0; index-- {
		sum := uint16(digest.Sum[index]) + uint16(hash[index]) + carry
		digest.Sum[index] = byte(sum)
		carry = sum >> 8
	}
	digest.Count++
	return nil
}

// RemoveStaging only removes files in one validated job directory. It never
// follows links or recursively removes arbitrary administrator-supplied paths.
func RemoveStaging(id string) error {
	directory, err := StagingDirectory(config.GetDBDir(), id)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unexpected non-file in staging directory: %s", entry.Name())
		}
		if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
			return err
		}
	}
	return os.Remove(directory)
}
