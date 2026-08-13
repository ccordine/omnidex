package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
)

func validateObjectiveCitationMetadata(record evidence.Record) error {
	web := record.SourceType == "web_document"
	expectedFields := 7
	if web {
		expectedFields = 10
	}
	if len(record.Metadata) != expectedFields {
		return fmt.Errorf(
			"objective citation metadata has %d fields; expected exactly %d",
			len(record.Metadata), expectedFields,
		)
	}
	for key := range record.Metadata {
		if objectiveCitationMetadataKeyAllowed(key, web) {
			continue
		}
		return fmt.Errorf("objective citation contains unknown metadata field %q", key)
	}
	if _, err := objectiveMetadataString(record.Metadata, "capsule_id", 128); err != nil {
		return err
	}
	if _, err := objectiveMetadataSHA(record.Metadata, "instruction_sha256"); err != nil {
		return err
	}
	if _, err := objectiveMetadataString(record.Metadata, "objective_id", 128); err != nil {
		return err
	}
	kind, err := objectiveMetadataString(record.Metadata, "objective_kind", 32)
	if err != nil {
		return err
	}
	requirementID, err := objectiveMetadataString(record.Metadata, "requirement_id", 128)
	if err != nil {
		return err
	}
	projectionSHA, err := objectiveMetadataSHA(record.Metadata, "projection_sha256")
	if err != nil {
		return err
	}
	projectionDigest := sha256.Sum256([]byte(record.Excerpt))
	if projectionSHA != hex.EncodeToString(projectionDigest[:]) {
		return fmt.Errorf("objective citation projection SHA-256 differs from its exact excerpt")
	}
	sourceSHA, err := objectiveMetadataSHA(record.Metadata, "source_sha256")
	if err != nil {
		return err
	}
	if sourceSHA != record.Hash {
		return fmt.Errorf("objective citation source SHA-256 differs from its evidence hash")
	}
	if web {
		if kind != "external_answer" {
			return fmt.Errorf("objective web citation requires objective kind %q", "external_answer")
		}
		return validateObjectiveWebMetadata(record, requirementID)
	}
	if kind != "repository_read" {
		return fmt.Errorf("objective non-web citation requires objective kind %q", "repository_read")
	}
	if len(record.SupportsClaims) != 1 || record.SupportsClaims[0] != requirementID {
		return fmt.Errorf("objective non-web citation claim differs from its requirement authority")
	}
	return nil
}

func objectiveCitationMetadataKeyAllowed(key string, web bool) bool {
	switch key {
	case "capsule_id", "instruction_sha256", "objective_id", "objective_kind",
		"requirement_id", "projection_sha256", "source_sha256":
		return true
	case "paragraph_indexes", "source_observed_at", "source_truncated":
		return web
	default:
		return false
	}
}

func validateObjectiveWebMetadata(record evidence.Record, requirementID string) error {
	rawIndexes, exists := record.Metadata["paragraph_indexes"]
	indexes, exactType := rawIndexes.([]int)
	if !exists || !exactType || len(indexes) < 1 || len(indexes) > 4 {
		return fmt.Errorf("objective web citation requires 1..4 exact integer paragraph indexes")
	}
	previous := -1
	for position, index := range indexes {
		if index < 0 || index > 3 || index <= previous {
			return fmt.Errorf("objective web citation paragraph indexes must be unique ascending values in 0..3")
		}
		if len(record.SupportsClaims) != len(indexes) ||
			record.SupportsClaims[position] != fmt.Sprintf("%s#paragraph-%d", requirementID, index+1) {
			return fmt.Errorf("objective web citation claims differ from paragraph authority")
		}
		previous = index
	}
	observed, err := objectiveMetadataString(record.Metadata, "source_observed_at", 35)
	if err != nil {
		return err
	}
	parsed, parseErr := time.Parse(time.RFC3339Nano, observed)
	if parseErr != nil || !strings.HasSuffix(observed, "Z") ||
		parsed.Location() != time.UTC || parsed.Format(time.RFC3339Nano) != observed {
		return fmt.Errorf("objective web citation observation must be exact canonical UTC RFC3339Nano")
	}
	if _, exists := record.Metadata["source_truncated"]; !exists {
		return fmt.Errorf("objective web citation requires source truncation authority")
	}
	if _, exactType := record.Metadata["source_truncated"].(bool); !exactType {
		return fmt.Errorf("objective web citation source truncation authority must be boolean")
	}
	return nil
}

func objectiveMetadataString(metadata map[string]any, key string, maximum int) (string, error) {
	raw, exists := metadata[key]
	value, exactType := raw.(string)
	if !exists || !exactType {
		return "", fmt.Errorf("objective citation metadata %q must be an exact string", key)
	}
	if strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("objective citation metadata %q must be one line", key)
	}
	if err := validateExactObjectiveEvidenceText("metadata "+key, value, maximum, true); err != nil {
		return "", err
	}
	return value, nil
}

func objectiveMetadataSHA(metadata map[string]any, key string) (string, error) {
	value, err := objectiveMetadataString(metadata, key, 64)
	if err != nil {
		return "", err
	}
	decoded, decodeErr := hex.DecodeString(value)
	if decodeErr != nil || len(decoded) != sha256.Size || value != strings.ToLower(value) {
		return "", fmt.Errorf("objective citation metadata %q requires an exact lowercase SHA-256", key)
	}
	return value, nil
}
