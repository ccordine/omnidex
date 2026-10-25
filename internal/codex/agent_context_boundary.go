package codex

import "encoding/json"

func contextWindowExceeded(raw json.RawMessage) bool {
	var name string
	if json.Unmarshal(raw, &name) == nil {
		return name == "contextWindowExceeded" || name == "ContextWindowExceeded"
	}
	var variants map[string]json.RawMessage
	if json.Unmarshal(raw, &variants) != nil {
		return false
	}
	_, camel := variants["contextWindowExceeded"]
	_, pascal := variants["ContextWindowExceeded"]
	return camel || pascal
}
