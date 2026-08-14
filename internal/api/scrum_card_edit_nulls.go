package api

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func validateScrumCardEditNulls(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	for _, name := range []string{
		"title", "description", "ref_files", "card_ticket",
		"card_prompt", "tags",
	} {
		if value, ok := fields[name]; ok && isJSONNull(value) {
			return fmt.Errorf("editable Scrum card field %q must not be null", name)
		}
	}
	for field, label := range map[string]string{"ref_files": "reference file", "tags": "tag"} {
		value, ok := fields[field]
		if !ok {
			continue
		}
		var items []json.RawMessage
		if err := json.Unmarshal(value, &items); err != nil {
			return err
		}
		for index, item := range items {
			if isJSONNull(item) {
				return fmt.Errorf("editable Scrum card %s %d must not be null", label, index)
			}
		}
	}
	return nil
}

func isJSONNull(raw []byte) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func validateScrumCardEditText(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	return rejectScrumCardNUL(value, "Scrum card editable patch")
}

func rejectScrumCardNUL(value any, path string) error {
	switch typed := value.(type) {
	case string:
		if bytes.IndexByte([]byte(typed), 0) >= 0 {
			return fmt.Errorf("%s contains a forbidden NUL character", path)
		}
	case []any:
		for index, item := range typed {
			if err := rejectScrumCardNUL(item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case map[string]any:
		for name, item := range typed {
			if err := rejectScrumCardNUL(item, path+"."+name); err != nil {
				return err
			}
		}
	}
	return nil
}
