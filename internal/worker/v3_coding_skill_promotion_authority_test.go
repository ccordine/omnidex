package worker

import (
	"os"
	"strings"
	"testing"
)

func TestCodingPipelineCannotCreateCertifyOrActivateSkillCandidates(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"v3_coding_driver_plan.go",
		"v3_coding_driver_verification.go",
		"v3_coding_runtime.go",
		"v3_coding_skill_retrieval.go",
		"v3_coding_skills.go",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{
			"createCodingSkillCandidate(",
			"NewSkillProcedureJob(",
			"CreateLearnedSkillCandidate",
			"StoreWorkerSkillEmbedding",
			"recordPendingSkillCheck(",
			"activatePendingSkills(",
			"ActivateWorkerSkillByStepAttempt(",
			"BeginWorkerSkillValidationByStepAttempt(",
			"RecordWorkerSkillCheckByStepAttempt(",
		} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s retains same-workload skill authority %q", path, forbidden)
			}
		}
	}
	raw, err := os.ReadFile("v3_coding_skill_retrieval.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	availability := strings.Index(source, "repo.HasActiveWorkerSkillEmbeddings(")
	embedding := strings.Index(source, ".embeddings.Embedding(")
	if availability < 0 || embedding < 0 || availability > embedding {
		t.Fatal("coding skill retrieval does not prove an active registry candidate before embedding")
	}
	for _, path := range []string{
		"../queue/worker_skill_lifecycle.go",
		"../queue/worker_skill_promotion.go",
		"../queue/step_attempt_skill_writes.go",
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("removed skill mutation surface still exists at %s: %v", path, err)
		}
	}
}
