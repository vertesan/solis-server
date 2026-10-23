package masterdata

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/proto"
)

// CustomTypes lists only the master tables changed (and reset) by a template.
// Source tables such as LoveHomeAction and Costume are never modified.
func CustomTypes(template string) ([]string, error) {
	switch template {
	case "home-action":
		return []string{mname.HomeAction}, nil
	case "photo":
		return []string{mname.PhotoActivity, mname.PhotoExpression, mname.PhotoQuestMusic, mname.PhotoQuestStage}, nil
	default:
		return nil, fmt.Errorf("unknown customization template %q; choose home-action or photo", template)
	}
}

// PrepareCustom streams the selected transformation into temporary YAML, then
// uses the normal import preparation for validation, encryption and semantic
// no-op detection. The entire photo template becomes one atomic publication.
func PrepareCustom(ctx context.Context, template string) (*ImportRequest, error) {
	types, err := CustomTypes(template)
	if err != nil {
		return nil, err
	}
	version, packs, err := activeManifest(ctx)
	if err != nil {
		return nil, err
	}
	source := mname.Costume
	if template == "home-action" {
		source = mname.LoveHomeAction
	}
	for _, name := range append(append([]string{}, types...), source) {
		if packs[name] == nil {
			return nil, fmt.Errorf("custom %s requires active master type %s", template, name)
		}
	}
	directory, err := os.MkdirTemp("", "solis-master-custom-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)

	var costumeTypes []string
	if template == "photo" {
		costumeTypes, err = customCostumeTypes(ctx)
		if err != nil {
			return nil, err
		}
	}
	files := make([]string, 0, len(types))
	for _, name := range types {
		path := filepath.Join(directory, name+".yaml")
		if err := writeCustomTable(ctx, path, version, packs[name], template, costumeTypes); err != nil {
			return nil, fmt.Errorf("customize %s: %w", name, err)
		}
		files = append(files, path)
	}
	if err := unchangedVersion(ctx, version); err != nil {
		return nil, err
	}
	return PrepareImport(ctx, ImportOptions{Files: files})
}

func writeCustomTable(ctx context.Context, path, version string, pack *ptransaction.MasterTagPack, template string, costumeTypes []string) (retErr error) {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(2)
	defer func() { retErr = errors.Join(retErr, encoder.Close(), file.Close()) }()
	if err := encoder.Encode(YAMLHeader{FormatVersion: 1, Type: pack.Type, BaseVersion: version, BaseFileName: pack.FileName}); err != nil {
		return err
	}
	if template == "home-action" {
		return writeCustomHomeActions(ctx, encoder)
	}
	return writeCustomPhotoTable(ctx, encoder, pack.Type, costumeTypes)
}

func decodeCustomRecord(data []byte, message proto.Message) error {
	if err := proto.Unmarshal(data, message); err != nil {
		return fmt.Errorf("invalid %s protobuf: %w", message.ProtoReflect().Descriptor().Name(), err)
	}
	// As in DecodeRecord, refuse newer unknown fields rather than silently
	// dropping them when the generated YAML is read back for publication.
	return rejectUnknownWire(message.ProtoReflect())
}

func writeCustomHomeActions(ctx context.Context, encoder *yaml.Encoder) error {
	// Retain only IDs in memory. Existing rows with those IDs are replaced,
	// while every unrelated home action is preserved unchanged.
	mapped := map[string]bool{}
	if err := opmaster.IterateMasterPayloads(ctx, mname.LoveHomeAction, func(data []byte) error {
		action := &pmaster.LoveHomeAction{}
		if err := decodeCustomRecord(data, action); err != nil {
			return err
		}
		if action.Id == "" || mapped[action.Id] {
			return fmt.Errorf("LoveHomeAction has an empty or duplicate id %q", action.Id)
		}
		mapped[action.Id] = true
		return writeRecord(encoder, &pmaster.HomeAction{
			HomeActionId:          action.Id,
			CharacterId:           action.CharacterId,
			MotionAssetId:         action.MotionAssetId,
			AdditionMotionAssetId: action.AdditionMotionAssetId,
			VoiceAssetId:          action.VoiceAssetId,
			Text:                  action.Text,
			ConditionId:           "",
			Weight:                100,
		})
	}); err != nil {
		return err
	}
	return opmaster.IterateMasterPayloads(ctx, mname.HomeAction, func(data []byte) error {
		action := &pmaster.HomeAction{}
		if err := decodeCustomRecord(data, action); err != nil {
			return err
		}
		if mapped[action.HomeActionId] {
			return nil
		}
		return writeRecord(encoder, action)
	})
}

func customCostumeTypes(ctx context.Context) ([]string, error) {
	seen := map[string]bool{}
	err := opmaster.IterateMasterPayloads(ctx, mname.Costume, func(data []byte) error {
		costume := &pmaster.Costume{}
		if err := decodeCustomRecord(data, costume); err != nil {
			return err
		}
		if costume.CostumeTypeId != "" {
			seen[costume.CostumeTypeId] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	types := make([]string, 0, len(seen))
	for name := range seen {
		types = append(types, name)
	}
	sort.Strings(types)
	return types, nil
}

func writeCustomPhotoTable(ctx context.Context, encoder *yaml.Encoder, name string, costumeTypes []string) error {
	return opmaster.IterateMasterPayloads(ctx, name, func(data []byte) error {
		var message proto.Message
		switch name {
		case mname.PhotoActivity:
			message = &pmaster.PhotoActivity{}
		case mname.PhotoExpression:
			message = &pmaster.PhotoExpression{}
		case mname.PhotoQuestMusic:
			message = &pmaster.PhotoQuestMusic{}
		case mname.PhotoQuestStage:
			message = &pmaster.PhotoQuestStage{}
		default:
			return fmt.Errorf("unsupported photo template master type %s", name)
		}
		if err := decodeCustomRecord(data, message); err != nil {
			return err
		}
		switch record := message.(type) {
		case *pmaster.PhotoActivity:
			record.ViewConditionId = ""
			record.ImpossibleShootingCharacterIds = nil
			// PhotoActivity has no impossiblePhotoPoseIds in the current schema.
			record.CostumeTypeIds = costumeTypes
		case *pmaster.PhotoExpression:
			record.ViewConditionId = ""
			record.ImpossibleShootingCharacterIds = nil
			record.ImpossiblePhotoPoseIds = nil
			record.CostumeTypeIds = costumeTypes
		case *pmaster.PhotoQuestMusic:
			record.ViewConditionId = ""
			record.UnlockConditionId = ""
		case *pmaster.PhotoQuestStage:
			record.ViewConditionId = ""
			record.UnlockConditionId = ""
		}
		return writeRecord(encoder, message)
	})
}
