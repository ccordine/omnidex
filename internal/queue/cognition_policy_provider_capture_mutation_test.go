package queue

import (
	"bytes"
	"encoding/json"
)

func mutateCapturedResponse(capture []byte) []byte {
	needle := []byte(`"response":"`)
	start := bytes.Index(capture, needle)
	if start < 0 || start+len(needle) >= len(capture) {
		return append(capture, 'x')
	}
	start += len(needle)
	if capture[start] == 'x' {
		capture[start] = 'y'
	} else {
		capture[start] = 'x'
	}
	return capture
}

func removeCapturedJSONField(raw []byte, name string) []byte {
	document, ok := decodeCapturedJSONObject(raw)
	if !ok {
		return raw
	}
	delete(document, name)
	return encodeCapturedJSONObject(raw, document)
}

func nullCapturedJSONField(raw []byte, name string) []byte {
	document, ok := decodeCapturedJSONObject(raw)
	if !ok {
		return raw
	}
	document[name] = json.RawMessage("null")
	return encodeCapturedJSONObject(raw, document)
}

func decodeCapturedJSONObject(raw []byte) (map[string]json.RawMessage, bool) {
	document := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, false
	}
	return document, true
}

func encodeCapturedJSONObject(original []byte, document map[string]json.RawMessage) []byte {
	encoded, err := json.Marshal(document)
	if err != nil {
		return original
	}
	return encoded
}
