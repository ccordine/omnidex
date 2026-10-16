package assemblyline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ProjectContextJSONText exposes decoded leaves, not JSON serialization syntax,
// to the ordinary prose context compiler. Artifact identities must already be
// bound by RedactContextArtifactJSON. Embedded serialized objects are projected
// recursively so neither escaping nor chunk boundaries invent path identities.
func ProjectContextJSONText(raw json.RawMessage) (string, error) {
	decode := func(raw []byte) (any, error) {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		err := decoder.Decode(&value)
		return value, err
	}
	value, err := decode(raw)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	var project func(string, any) error
	project = func(label string, value any) error {
		switch typed := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if err := project(label+"."+key, typed[key]); err != nil {
					return err
				}
			}
			if len(keys) > 0 {
				return nil
			}
		case []any:
			for index, child := range typed {
				if err := project(fmt.Sprintf("%s[%d]", label, index), child); err != nil {
					return err
				}
			}
			if len(typed) > 0 {
				return nil
			}
		case string:
			if compoundContextJSON(typed) {
				child, err := decode([]byte(typed))
				if err != nil {
					return err
				}
				return project(label, child)
			}
			fmt.Fprintf(&output, "%s:\n%s\n\n", label, typed)
			return nil
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		fmt.Fprintf(&output, "%s: %s\n\n", label, encoded)
		return nil
	}
	if err := project("context", value); err != nil {
		return "", err
	}
	return strings.TrimSpace(output.String()), nil
}
