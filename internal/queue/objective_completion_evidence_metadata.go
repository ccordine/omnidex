package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/roleplay"
)

var objectiveRoleplayResearchMetadataKeys = []string{
	"authority_namespace",
	"roleplay_research_preparation_id",
	"roleplay_research_world_id",
	"roleplay_research_character_id",
	"roleplay_research_question_sha256",
	"roleplay_research_capability_grant_id",
}

func validateObjectiveCitationMetadata(record evidence.Record) error {
	web := record.SourceType == "web_document"
	database := record.SourceType == "postgres_query"
	roleplayResearch := hasAnyObjectiveRoleplayResearchMetadata(record.Metadata)
	if roleplayResearch && !web {
		return fmt.Errorf("roleplay research authority is valid only for real-world web evidence")
	}
	expectedFields := 7
	if web {
		expectedFields = 10
	} else if database {
		expectedFields = 8
	}
	if roleplayResearch {
		expectedFields += len(objectiveRoleplayResearchMetadataKeys)
	}
	if len(record.Metadata) != expectedFields {
		return fmt.Errorf(
			"objective citation metadata has %d fields; expected exactly %d",
			len(record.Metadata), expectedFields,
		)
	}
	for key := range record.Metadata {
		if objectiveCitationMetadataKeyAllowed(key, web, database, roleplayResearch) {
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
		if err := validateObjectiveWebMetadata(record, requirementID); err != nil {
			return err
		}
		if roleplayResearch {
			return validateObjectiveRoleplayResearchMetadata(record.Metadata)
		}
		return nil
	}
	if database {
		if kind != "database_read" {
			return fmt.Errorf("objective database citation requires objective kind %q", "database_read")
		}
		if err := validateObjectiveDatabaseMetadata(record); err != nil {
			return err
		}
		if len(record.RequirementAuthorityBindings) != 1 ||
			record.RequirementAuthorityBindings[0] != requirementID {
			return fmt.Errorf("objective database citation binding differs from its requirement authority")
		}
		return nil
	}
	if kind != "repository_read" {
		return fmt.Errorf("objective non-web citation requires objective kind %q", "repository_read")
	}
	if len(record.RequirementAuthorityBindings) != 1 ||
		record.RequirementAuthorityBindings[0] != requirementID {
		return fmt.Errorf("objective non-web citation binding differs from its requirement authority")
	}
	return nil
}

func objectiveCitationMetadataKeyAllowed(key string, web, database, roleplayResearch bool) bool {
	switch key {
	case "capsule_id", "instruction_sha256", "objective_id", "objective_kind",
		"requirement_id", "projection_sha256", "source_sha256":
		return true
	case "paragraph_indexes", "source_observed_at", "source_truncated":
		return web
	case "source_acquired_at":
		return database
	case "authority_namespace", "roleplay_research_preparation_id",
		"roleplay_research_world_id", "roleplay_research_character_id",
		"roleplay_research_question_sha256", "roleplay_research_capability_grant_id":
		return roleplayResearch
	default:
		return false
	}
}

func hasAnyObjectiveRoleplayResearchMetadata(metadata map[string]any) bool {
	for _, key := range objectiveRoleplayResearchMetadataKeys {
		if _, exists := metadata[key]; exists {
			return true
		}
	}
	return false
}

func validateObjectiveRoleplayResearchMetadata(metadata map[string]any) error {
	namespace, err := objectiveMetadataString(metadata, "authority_namespace", 32)
	if err != nil {
		return err
	}
	if namespace != string(roleplay.AuthorityRealWorld) {
		return fmt.Errorf("roleplay research citation requires REAL_WORLD authority")
	}
	for _, key := range []string{
		"roleplay_research_preparation_id", "roleplay_research_world_id",
		"roleplay_research_character_id", "roleplay_research_capability_grant_id",
	} {
		if _, err := objectiveMetadataString(metadata, key, 128); err != nil {
			return err
		}
	}
	_, err = objectiveMetadataSHA(metadata, "roleplay_research_question_sha256")
	return err
}

func validateObjectiveDatabaseMetadata(record evidence.Record) error {
	acquired, err := objectiveMetadataString(record.Metadata, "source_acquired_at", 35)
	if err != nil {
		return err
	}
	parsed, parseErr := time.Parse(time.RFC3339Nano, acquired)
	if parseErr != nil || !strings.HasSuffix(acquired, "Z") ||
		parsed.Location() != time.UTC || parsed.Format(time.RFC3339Nano) != acquired {
		return fmt.Errorf("objective database citation acquisition must be exact canonical UTC RFC3339Nano")
	}
	return nil
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
		if len(record.RequirementAuthorityBindings) != len(indexes) ||
			record.RequirementAuthorityBindings[position] !=
				fmt.Sprintf("%s#paragraph-%d", requirementID, index+1) {
			return fmt.Errorf("objective web citation bindings differ from paragraph authority")
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
