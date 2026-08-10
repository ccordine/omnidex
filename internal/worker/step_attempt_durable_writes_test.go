package worker

import (
	"os"
	"strings"
	"testing"
)

func TestWorkerDurableWritesUseExactStepAttemptAPIs(t *testing.T) {
	t.Parallel()
	checks := map[string][]string{
		"data_source_explore.go": {
			"repo.SaveDataSourceCatalog(", "repo.UpdateDataSourceCatalogTimestamp(",
		},
		"data_source_query.go": {
			"repo.UpdateDataSourceCatalogTimestamp(", "repo.AddDataSourceChannelMessage(",
		},
		"data_source_memory.go": {"repo.AddMemoryChunk("},
		"project_debugger.go": {
			"repo.CreateProjectDebuggerCardJob(", "repo.UpdateProjectSetting(",
		},
		"scrum_card_llm.go": {"repo.UpdateScrumCard("},
		"telemetry_llm.go": {
			"repo.RecordLLMContextUsage(", "repo.RecordTelemetryModelCall(",
		},
		"v3_coding_skill_retrieval.go": {
			"repo.CreateLearnedSkillCandidate(", "repo.StoreWorkerSkillEmbedding(",
			"repo.BeginWorkerSkillValidation(",
		},
		"v3_coding_skills.go": {
			"repo.RecordWorkerSkillCheck(", "repo.ActivateWorkerSkill(",
			"repo.RejectWorkerSkill(",
		},
	}
	for path, forbidden := range checks {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range forbidden {
			if strings.Contains(string(raw), call) {
				t.Errorf("%s retains unfenced worker write %q", path, call)
			}
		}
	}
}
