package worker

import (
	"os"
	"strings"
	"testing"
)

func TestWorkerDurableWritesUseExactStepAttemptAPIs(t *testing.T) {
	t.Parallel()
	checks := map[string][]string{
		"telemetry_llm.go": {
			"repo.RecordLLMContextUsage(", "repo.RecordTelemetryModelCall(",
		},
		"v3_coding_skill_retrieval.go": {
			"repo.CreateLearnedSkillCandidate", "repo.StoreWorkerSkillEmbedding",
			"repo.BeginWorkerSkillValidation", "repo.PromoteWorkerSkill",
		},
		"v3_coding_skills.go": {
			"repo.RecordWorkerSkillCheck", "repo.ActivateWorkerSkill",
			"repo.RejectWorkerSkill", "NewSkillProcedureJob",
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
