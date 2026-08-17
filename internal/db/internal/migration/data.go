package migration

import (
	"embed"
	"encoding/json"
	"fmt"

	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// migrationFiles stores only the embedded file index. ReadFile allocates the
// requested YAML payload locally so one-time migration data can be reclaimed.
//
//go:embed catalogdata/*.yaml initdata/*.yaml
var migrationFiles embed.FS

func unmarshalMigrationYAML(path string, target any) error {
	data, err := migrationFiles.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read migration data %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to decode migration data %q: %w", path, err)
	}
	return nil
}

func unmarshalMigrationProtoYAML(path string, message proto.Message) error {
	var snapshot map[string]any
	if err := unmarshalMigrationYAML(path, &snapshot); err != nil {
		return err
	}
	protobufJSON, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("failed to normalize migration data %q: %w", path, err)
	}
	if err := protojson.Unmarshal(protobufJSON, message); err != nil {
		return fmt.Errorf("failed to decode protobuf migration data %q: %w", path, err)
	}
	return nil
}
