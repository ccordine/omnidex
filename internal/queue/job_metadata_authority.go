package queue

import "fmt"

var removedJobMetadataKeys = []string{
	"runtime",
	"engine",
	"execution_mode",
	"v3_enabled",
	"scrum_current_user_instruction",
	"v3_authority_directives",
	"persistent_execution",
	"planning_passes",
	"review_always",
	"allow_missing_tools",
	"reasoning_level",
	"autonomy_mode",
	"approval_mode",
	"verification_mode",
	"verification_iterations",
	"architect_mode",
	"web_search",
	"workspace_scan",
}

func ValidateJobMetadataAuthority(metadata map[string]any) error {
	for _, key := range removedJobMetadataKeys {
		if _, removed := metadata[key]; removed {
			return fmt.Errorf(
				"job metadata key %s was removed; the authoritative runtime rejects legacy and write-only controls",
				key,
			)
		}
	}
	return nil
}
