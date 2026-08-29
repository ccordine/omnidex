package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRetiredExecutionAuthorityHasNoOperationalSourcePath(t *testing.T) {
	t.Parallel()
	for path, forbidden := range map[string][]string{
		"../model/project.go": {
			"RecipeID", `json:"recipe_id`, `json:"recipe`,
		},
		"projects.go": {
			"recipe_id", "RecipeID", ".Recipe", "agent_config",
		},
		"scrum_card_record.go": {
			"agent_config", "recipe_id", "RecipeID", ".Recipe",
			"TagsJobID", "TicketJobID", "tags_job_id", "ticket_job_id",
		},
		"scrum_card_selection.go": {
			"agent_config", "recipe_id", "RecipeID", ".Recipe",
			"tags_job_id", "ticket_job_id",
		},
		"scrum_card_scan.go": {
			"TagsJobID", "TicketJobID",
		},
		"scrum_card_mutation.go": {
			"TagsJobID", "TicketJobID", "tags_job_id", "ticket_job_id",
		},
		"scrum_card_state_validation.go": {
			"TagsJobID", "TicketJobID", "tags_job_id", "ticket_job_id",
		},
		"repository.go": {
			`firstMetadataString(metadata, "recipe_id")`,
			`metadataStringSlice(metadata, "external_agents_used")`,
		},
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, needle := range forbidden {
			if strings.Contains(string(raw), needle) {
				t.Errorf("retired operational authority %q remains in %s", needle, path)
			}
		}
	}
	for _, path := range []string{
		"../agentconfig/config.go",
		"../agentconfig/parse.go",
		"../agentconfig/resolve.go",
		"agent_settings.go",
		"scrum_card_config_validation.go",
		"../api/agent_config_service.go",
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			if err != nil {
				t.Fatalf("inspect retired source %s: %v", path, err)
			}
			t.Errorf("retired operational source remains: %s", path)
		}
	}
}

func TestRetiredExecutionTelemetryHasNoCompatibilityDTOOrQuery(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"telemetry.go",
		"telemetry_queries.go",
		"repository.go",
		"repository_job_telemetry.go",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, forbidden := range []string{
			"RecipeID", "recipe_id", "ExternalAgentsUsed", "external_agents_used",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("retired telemetry compatibility authority %q remains in %s", forbidden, path)
			}
		}
	}
}
