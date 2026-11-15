package agentmgr

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// DecodeMode says how strictly a reply payload is decoded.
type DecodeMode int

const (
	// DecodeLenient is json.Unmarshal: unknown fields are ignored.
	DecodeLenient DecodeMode = iota
	// DecodeStrict refuses unknown fields and anything after the one object.
	DecodeStrict
)

var errTrailingPayload = errors.New("agent payload must contain exactly one object")

// DecodeData decodes one reply payload into dst.
func DecodeData(data []byte, dst any, mode DecodeMode) error {
	if mode != DecodeStrict {
		return json.Unmarshal(data, dst)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errTrailingPayload
	}
	return nil
}
