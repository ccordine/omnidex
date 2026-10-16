package assemblyline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/modelcontext"
)

// Structured context must be decoded before path recognition. JSON's escaping
// syntax (including nested serialized ledgers) is not a Windows filesystem path.
func RedactContextArtifactJSON(raw json.RawMessage) (string, []ArtifactIdentity, error) {
	identities := []ArtifactIdentity{}
	tokens := map[string]string{}
	redacted, err := mapContextJSON(raw, func(value string) (string, error) {
		matches := modelcontext.PathIdentities(value, modelcontext.ArtifactIdentityProvenance{})
		var result strings.Builder
		previous := 0
		for _, match := range matches {
			token, exists := tokens[match.Value]
			if !exists {
				token = fmt.Sprintf("ARTIFACT_%d", len(identities)+1)
				tokens[match.Value] = token
				identities = append(identities, ArtifactIdentity{Token: token, Value: match.Value})
			}
			result.WriteString(value[previous:match.Start])
			result.WriteString(token)
			previous = match.End
		}
		result.WriteString(value[previous:])
		return result.String(), nil
	})
	return string(redacted), identities, err
}

func RestoreContextArtifactJSONOrText(value string, identities []ArtifactIdentity) (string, error) {
	if !json.Valid([]byte(value)) {
		return RestoreArtifactIdentities(value, identities)
	}
	restored, err := mapContextJSON([]byte(value), func(text string) (string, error) {
		return RestoreArtifactIdentities(text, identities)
	})
	return string(restored), err
}

func compoundContextJSON(value string) bool {
	trimmed := strings.TrimSpace(value)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid([]byte(trimmed))
}

func mapContextJSON(raw json.RawMessage, transform func(string) (string, error)) (json.RawMessage, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var walk func(any) (any, error)
	walk = func(value any) (any, error) {
		switch typed := value.(type) {
		case string:
			if compoundContextJSON(typed) {
				encoded, err := mapContextJSON([]byte(typed), transform)
				return string(encoded), err
			}
			return transform(typed)
		case []any:
			for index, item := range typed {
				mapped, err := walk(item)
				if err != nil {
					return nil, err
				}
				typed[index] = mapped
			}
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				mapped, err := walk(typed[key])
				if err != nil {
					return nil, err
				}
				typed[key] = mapped
			}
		}
		return value, nil
	}
	mapped, err := walk(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(mapped)
}
