package agentmgr

import "encoding/json"

// NewMessage builds the envelope for one message to an agent: payload goes
// through json.Marshal into Data. A nil payload sends no data field, as a
// message literal without Data did; a typed nil still encodes as null.
//
// On an encode error the message still carries Type and ID with no data, and
// the error is json.Marshal's, unwrapped. A caller that ignored the error
// before (data, _ := json.Marshal) sends exactly what it sent then.
func NewMessage(msgType, id string, payload any) (Message, error) {
	msg := Message{Type: msgType, ID: id}
	if payload == nil {
		return msg, nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return msg, err
	}
	msg.Data = data
	return msg, nil
}

// Decode decodes msg's payload into a new T as json.Unmarshal does: unknown
// fields are ignored. On error it returns what was decoded before the error,
// as json.Unmarshal leaves its target, and json.Unmarshal's error, unwrapped.
func Decode[T any](msg Message) (T, error) {
	var value T
	err := DecodeData(msg.Data, &value, DecodeLenient)
	return value, err
}

// DecodeStrictly is Decode with DecodeStrict rules: an unknown field or
// anything after the one object is an error.
func DecodeStrictly[T any](msg Message) (T, error) {
	var value T
	err := DecodeData(msg.Data, &value, DecodeStrict)
	return value, err
}
