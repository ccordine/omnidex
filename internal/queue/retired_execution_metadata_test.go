package queue

import "testing"

func TestJobMetadataRejectsRetiredAgentAndRecipeAuthority(t *testing.T) {
	t.Parallel()
	for _, key := range []string{
		"agent_config",
		"agent_config_source",
		"instance_agent_config",
		"external_agents_used",
		"execution_agent",
		"agent_strict",
		"scrum_raw_play",
		"omnidex_no_delegate",
		"recipe_id",
		"recipe",
	} {
		metadata := map[string]any{key: true}
		if err := ValidateJobMetadataAuthority(metadata); err == nil {
			t.Errorf("generic job accepted retired metadata key %q", key)
		}
	}
}
