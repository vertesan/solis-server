package diver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	goccyjson "github.com/goccy/go-json"
)

// JSONErrorDescription preserves useful decoding diagnostics without including
// rejected values, which may contain a password, token, or other request secret.
func JSONErrorDescription(err error) string {
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return fmt.Sprintf("invalid JSON syntax at byte %d", syntax.Offset)
	}
	var fastSyntax *goccyjson.SyntaxError
	if errors.As(err, &fastSyntax) {
		return fmt.Sprintf("invalid JSON syntax at byte %d", fastSyntax.Offset)
	}
	var mismatch *json.UnmarshalTypeError
	if errors.As(err, &mismatch) {
		return fmt.Sprintf("invalid JSON type for field %q at byte %d (expected %v)", mismatch.Field, mismatch.Offset, mismatch.Type)
	}
	var fastMismatch *goccyjson.UnmarshalTypeError
	if errors.As(err, &fastMismatch) {
		return fmt.Sprintf("invalid JSON type for field %q at byte %d (expected %v)", fastMismatch.Field, fastMismatch.Offset, fastMismatch.Type)
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "incomplete JSON input"
	}
	return fmt.Sprintf("JSON decoding failed (%T)", err)
}
