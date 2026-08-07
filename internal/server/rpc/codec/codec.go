package codec

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
)

func messageV2Of(v any) proto.Message {
	switch v := v.(type) {
	case protoadapt.MessageV1:
		return protoadapt.MessageV2Of(v)
	case protoadapt.MessageV2:
		return v
	}
	return nil
}

type QuaClientCodec struct{}

func (g *QuaClientCodec) Marshal(v any) ([]byte, error) {
	vv := messageV2Of(v)
	if vv == nil {
		return nil, fmt.Errorf("failed to marshal, message is %T, want proto.Message", v)
	}
	// default codec
	protoBytes, err := proto.Marshal(vv)
	if err != nil {
		return nil, err
	}
	// customized codec
	encBytes, err := Serialize(protoBytes, CLIENT)
	if err != nil {
		return nil, err
	}
	return encBytes, nil
}

func (g *QuaClientCodec) Unmarshal(data []byte, v any) error {
	vv := messageV2Of(v)
	if vv == nil {
		return fmt.Errorf("failed to unmarshal, message is %T, want proto.Message", v)
	}
	// customized codec
	protoBytes, err := Deserialize(data)
	if err != nil {
		return err
	}
	// default codec
	return proto.Unmarshal(protoBytes, vv)
}

func (g *QuaClientCodec) Name() string {
	return "qua-client"
}

type QuaServerCodec struct{}

func (g *QuaServerCodec) Marshal(v any) ([]byte, error) {
	vv := messageV2Of(v)
	if vv == nil {
		return nil, fmt.Errorf("failed to marshal, message is %T, want proto.Message", v)
	}
	// default codec
	protoBytes, err := proto.Marshal(vv)
	if err != nil {
		return nil, err
	}
	// customized codec
	encBytes, err := Serialize(protoBytes, SERVER)
	if err != nil {
		return nil, err
	}
	return encBytes, nil
}

func (g *QuaServerCodec) Unmarshal(data []byte, v any) error {
	vv := messageV2Of(v)
	if vv == nil {
		return fmt.Errorf("failed to unmarshal, message is %T, want proto.Message", v)
	}
	// customized codec
	protoBytes, err := Deserialize(data)
	if err != nil {
		return err
	}
	// default codec
	return proto.Unmarshal(protoBytes, vv)
}

func (g *QuaServerCodec) Name() string {
	return "qua-server"
}
