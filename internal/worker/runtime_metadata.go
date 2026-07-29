package worker

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func metadataValue(metadata json.RawMessage, key string) (any, bool) {
	if len(metadata) == 0 {
		return nil, false
	}
	var values map[string]any
	if err := json.Unmarshal(metadata, &values); err != nil {
		return nil, false
	}
	value, exists := values[key]
	return value, exists
}

func metadataString(metadata json.RawMessage, key string) string {
	value, exists := metadataValue(metadata, key)
	if !exists {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func metadataModel(job model.Job, key, fallback string) string {
	if configured := metadataString(job.Metadata, key); configured != "" {
		return configured
	}
	return strings.TrimSpace(fallback)
}

func specialistRoleForJob(job model.Job, defaultRoleID string) string {
	if configured := metadataString(job.Metadata, "specialist_role_id"); configured != "" {
		return configured
	}
	return strings.TrimSpace(defaultRoleID)
}

func specialistModel(job model.Job, defaultRoleID, fallback string, routing ModelRouting) string {
	roleID := specialistRoleForJob(job, defaultRoleID)
	if configured := strings.TrimSpace(routing.Specialist[roleID]); configured != "" {
		return configured
	}
	return strings.TrimSpace(fallback)
}

func clientCWDForJob(job model.Job) string {
	return metadataString(job.Metadata, "client_cwd")
}

func sessionTag(job model.Job) string {
	value := normalizeScopeID(metadataString(job.Metadata, "session_id"))
	if value == "" {
		return ""
	}
	return "session:" + value
}

func projectTag(job model.Job) string {
	location := clientCWDForJob(job)
	if location == "" {
		location = metadataString(job.Metadata, "host_env_cwd")
	}
	if location == "" {
		return ""
	}
	clean := filepath.Clean(location)
	base := normalizeScopeID(filepath.Base(clean))
	if base == "" {
		base = "workspace"
	}
	sum := sha1.Sum([]byte(strings.ToLower(clean)))
	return "project:" + base + "-" + hex.EncodeToString(sum[:4])
}

func memoryScopeTags(job model.Job, base []string) []string {
	tags := appendUnique(nil, base...)
	if project := projectTag(job); project != "" {
		tags = appendUnique([]string{project}, tags...)
	}
	if session := sessionTag(job); session != "" {
		tags = appendUnique([]string{session}, tags...)
	}
	return tags
}

func normalizeScopeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var output strings.Builder
	previousSeparator := false
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			output.WriteRune(character)
			previousSeparator = false
			continue
		}
		if output.Len() > 0 && !previousSeparator {
			output.WriteByte('-')
			previousSeparator = true
		}
	}
	result := strings.Trim(output.String(), "-")
	if len(result) > 64 {
		result = strings.Trim(result[:64], "-")
	}
	return result
}
