package masterdata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type YAMLHeader struct {
	FormatVersion int    `json:"formatVersion" yaml:"formatVersion"`
	Type          string `json:"type" yaml:"type"`
	BaseVersion   string `json:"baseVersion" yaml:"baseVersion"`
	BaseFileName  string `json:"baseFileName" yaml:"baseFileName"`
}

type YAMLReader struct {
	Header     YAMLHeader
	decoder    *yaml.Decoder
	descriptor protoreflect.MessageDescriptor
	record     int64
}

func masterDescriptor(name string) (protoreflect.MessageDescriptor, error) {
	descriptor := pmaster.File_pmaster_proto.Messages().ByName(protoreflect.Name(name))
	if descriptor == nil {
		return nil, fmt.Errorf("unknown master protobuf type %q", name)
	}
	return descriptor, nil
}

func NewYAMLReader(reader io.Reader) (*YAMLReader, error) {
	decoder := yaml.NewDecoder(reader)
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil {
		return nil, fmt.Errorf("read master header: %w", err)
	}
	data, err := yamlJSON(&node)
	if err != nil {
		return nil, fmt.Errorf("master header: %w", err)
	}
	var header YAMLHeader
	jsonDecoder := json.NewDecoder(bytes.NewReader(data))
	jsonDecoder.DisallowUnknownFields()
	if err := jsonDecoder.Decode(&header); err != nil {
		return nil, fmt.Errorf("master header: %w", err)
	}
	if header.FormatVersion != 1 || header.BaseVersion == "" || !encryptedFileName.MatchString(header.BaseFileName) {
		return nil, errors.New("master header requires formatVersion 1, baseVersion and a valid baseFileName")
	}
	descriptor, err := masterDescriptor(header.Type)
	if err != nil {
		return nil, err
	}
	return &YAMLReader{Header: header, decoder: decoder, descriptor: descriptor}, nil
}

// Next decodes exactly one document. protojson validates protobuf field names,
// scalar ranges, enum names, repeated fields and nested message structure.
func (reader *YAMLReader) Next() (proto.Message, error) {
	var node yaml.Node
	if err := reader.decoder.Decode(&node); err != nil {
		return nil, err
	}
	reader.record++
	data, err := yamlJSON(&node)
	if err != nil {
		return nil, fmt.Errorf("record %d: %w", reader.record, err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	if len(envelope) != 1 || envelope["data"] == nil || bytes.Equal(envelope["data"], []byte("null")) {
		return nil, fmt.Errorf("record %d must contain only a data object", reader.record)
	}
	message := dynamicpb.NewMessage(reader.descriptor)
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(envelope["data"], message); err != nil {
		return nil, fmt.Errorf("record %d: %w", reader.record, err)
	}
	return message, nil
}

// DecodeRecord refuses unknown wire fields recursively. Otherwise an export
// followed by import would silently erase fields from a newer master schema.
func DecodeRecord(masterType string, data []byte) (proto.Message, error) {
	descriptor, err := masterDescriptor(masterType)
	if err != nil {
		return nil, err
	}
	message := dynamicpb.NewMessage(descriptor)
	if err := proto.Unmarshal(data, message); err != nil {
		return nil, fmt.Errorf("invalid %s protobuf: %w", masterType, err)
	}
	if err := rejectUnknownWire(message.ProtoReflect()); err != nil {
		return nil, err
	}
	return message, nil
}

func rejectUnknownWire(message protoreflect.Message) error {
	if len(message.GetUnknown()) != 0 {
		return fmt.Errorf("%s contains unsupported protobuf fields", message.Descriptor().FullName())
	}
	var result error
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.IsMap() {
			if field.MapValue().Kind() == protoreflect.MessageKind {
				value.Map().Range(func(_ protoreflect.MapKey, element protoreflect.Value) bool {
					result = rejectUnknownWire(element.Message())
					return result == nil
				})
			}
		} else if field.Kind() == protoreflect.MessageKind || field.Kind() == protoreflect.GroupKind {
			if field.IsList() {
				for i := 0; i < value.List().Len() && result == nil; i++ {
					result = rejectUnknownWire(value.List().Get(i).Message())
				}
			} else {
				result = rejectUnknownWire(value.Message())
			}
		}
		return result == nil
	})
	return result
}

func writeRecord(encoder *yaml.Encoder, message proto.Message) error {
	data, err := (protojson.MarshalOptions{UseProtoNames: false}).Marshal(message)
	if err != nil {
		return err
	}
	var node yaml.Node
	// JSON is valid YAML: this preserves quoted protobuf int64 values instead
	// of passing through float64, including values above 2^53.
	if err := yaml.Unmarshal(data, &node); err != nil {
		return err
	}
	var blockStyle func(*yaml.Node)
	blockStyle = func(node *yaml.Node) {
		if node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode {
			node.Style = 0
		}
		for _, child := range node.Content {
			blockStyle(child)
		}
	}
	blockStyle(&node)
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "data"}, node.Content[0]}}
	return encoder.Encode(root)
}

func yamlJSON(node *yaml.Node) ([]byte, error) {
	var output bytes.Buffer
	if err := appendYAMLJSON(&output, node, 0); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func appendYAMLJSON(output *bytes.Buffer, node *yaml.Node, depth int) error {
	if depth > 100 {
		return errors.New("YAML nesting exceeds 100 levels")
	}
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) != 1 {
			return errors.New("empty YAML document")
		}
		return appendYAMLJSON(output, node.Content[0], depth+1)
	}
	switch node.Kind {
	case yaml.MappingNode:
		output.WriteByte('{')
		seen := map[string]bool{}
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || seen[key.Value] {
				return errors.New("YAML object keys must be unique strings")
			}
			seen[key.Value] = true
			if index > 0 {
				output.WriteByte(',')
			}
			encoded, _ := json.Marshal(key.Value)
			output.Write(encoded)
			output.WriteByte(':')
			if err := appendYAMLJSON(output, node.Content[index+1], depth+1); err != nil {
				return err
			}
		}
		output.WriteByte('}')
	case yaml.SequenceNode:
		output.WriteByte('[')
		for index, child := range node.Content {
			if index > 0 {
				output.WriteByte(',')
			}
			if err := appendYAMLJSON(output, child, depth+1); err != nil {
				return err
			}
		}
		output.WriteByte(']')
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str":
			encoded, _ := json.Marshal(node.Value)
			output.Write(encoded)
		case "!!null":
			output.WriteString("null")
		case "!!bool":
			output.WriteString(strings.ToLower(node.Value))
		case "!!int":
			// Accept decimal only: JSON has no octal/hex syntax. Keeping the
			// original digits avoids rounding any signed/unsigned 64-bit value.
			value := node.Value
			if strings.ContainsAny(value, "_xXoObB+") {
				return errors.New("integer values must use decimal notation")
			}
			if _, err := strconv.ParseInt(value, 10, 64); err != nil {
				if _, unsignedErr := strconv.ParseUint(value, 10, 64); unsignedErr != nil {
					return fmt.Errorf("invalid integer %q", value)
				}
			}
			if len(value) > 1 && (value[0] == '0' || (len(value) > 2 && value[0] == '-' && value[1] == '0')) {
				return errors.New("integer values must not have leading zeroes")
			}
			output.WriteString(value)
		case "!!float":
			if !json.Valid([]byte(node.Value)) {
				return errors.New("floating-point values must be finite JSON numbers")
			}
			output.WriteString(node.Value)
		default:
			return fmt.Errorf("unsupported YAML tag %q", node.Tag)
		}
	default:
		return errors.New("YAML aliases and custom nodes are unsupported")
	}
	return nil
}
