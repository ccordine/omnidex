package queue

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestGenerationSensitiveWritersRequireCurrentAuthority(t *testing.T) {
	cases := map[string][]string{
		"repository_current_records.go": {
			"requireActiveStepAttemptTx",
			"steps.current_attempt=$8",
		},
		"repository_steps.go": {
			"lockStepAttemptAuthorityTx",
			"superseded_at_generation IS NULL",
		},
		"repository_step_input.go": {
			"requireActiveStepAttemptTx",
			"job_steps.superseded_at_generation IS NULL",
			"job_steps.generation = jobs.current_generation",
		},
		"repository_step_output.go": {
			"requireActiveStepAttemptTx",
			"superseded_at_generation IS NULL",
		},
		"repository_step_context.go": {
			"requireActiveStepAttemptTx",
			"steps.generation=jobs.current_generation",
		},
		"step_attempt_authority.go": {
			"ErrStaleStepAttempt",
			"currentAttempt != authority.Attempt",
			"attemptWorker != authority.WorkerID",
		},
		"repository_job_reads.go": {
			"IsoLevel: pgx.RepeatableRead",
			"AccessMode: pgx.ReadOnly",
		},
		"repository_step_claim.go": {
			"FOR UPDATE OF jobs SKIP LOCKED",
			"superseded_at_generation IS NULL",
			"generation=jobs.current_generation",
		},
		"repository_step_projection.go": {
			"maxClaimedStepContextItems",
			"maxClaimedStepContextBytes",
			"ErrContextProjectionBudget",
			"LIMIT $4",
		},
	}
	for path, required := range cases {
		t.Run(path, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, token := range required {
				if !strings.Contains(string(raw), token) {
					t.Fatalf("%s omitted current-generation authority %q", path, token)
				}
			}
		})
	}
}

func TestTaskCommandGenerationValidationPrecedesNewMutation(t *testing.T) {
	raw, err := os.ReadFile("task_ledger_apply.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	replay := strings.Index(source, "loadTaskEventByCommandTx")
	generation := strings.Index(source, "validateTaskCommandGenerationTx")
	apply := strings.Index(source, "ledger.Apply(command)")
	if replay < 0 || generation < 0 || apply < 0 || !(replay < generation && generation < apply) {
		t.Fatalf("task command ordering replay=%d generation=%d apply=%d", replay, generation, apply)
	}
}

func TestQueueExposesNoAmbiguousGenerationReadNames(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var source strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		source.Write(raw)
	}
	for _, name := range []string{
		"GetJobDetails", "LatestArtifact", "ListArtifactsByJob", "ListEvidenceByJob",
		"ListMemoryCandidates",
	} {
		pattern := regexp.MustCompile(`func\s+\([^)]*\)\s+` + name + `\s*\(`)
		if pattern.MatchString(source.String()) {
			t.Fatalf("queue still exposes generation-ambiguous method %s", name)
		}
	}
}

func TestJobMemoryCandidateRequiresObservedGeneration(t *testing.T) {
	candidate := model.MemoryCandidate{
		Scope: model.MemoryScope{ProjectID: 1, ChannelID: "channel-one"},
		JobID: 9, CandidateKind: "reference", Content: "bounded",
	}
	if _, err := (&Repository{}).WriteMemoryCandidate(context.Background(), candidate); err == nil ||
		!strings.Contains(err.Error(), "observed positive generation") {
		t.Fatalf("missing generation error=%v", err)
	}
	generation := int64(1)
	candidate.JobID = 0
	candidate.Generation = &generation
	if _, err := (&Repository{}).WriteMemoryCandidate(context.Background(), candidate); err == nil ||
		!strings.Contains(err.Error(), "global memory candidate") {
		t.Fatalf("global generation error=%v", err)
	}

	raw, err := os.ReadFile("repository_memory_candidate_writes.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "lockObservedJobGenerationTx") ||
		!strings.Contains(string(raw), "FOR UPDATE") ||
		!strings.Contains(string(raw), "ErrStaleJobGeneration") {
		t.Fatal("job memory candidate write is not bound to the observed current generation")
	}
}
