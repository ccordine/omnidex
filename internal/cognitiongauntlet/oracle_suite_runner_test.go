package cognitiongauntlet

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestInitialOracleSuiteRunnerExecutesAndSealsExactlyFiveSuites(t *testing.T) {
	request := oracleSuiteTestRequest(t)
	report, err := RunInitialOracleGauntlets(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, result := range report.Results {
		caseID := result.Authority.CaseID
		if _, err := os.Stat(filepath.Join(request.EpisodeRoot, caseID, "episode.json")); err != nil {
			t.Fatalf("case %s episode: %v", caseID, err)
		}
		if _, err := os.Stat(filepath.Join(request.EvaluationRoot, caseID, "evaluation.json")); err != nil {
			t.Fatalf("case %s evaluation: %v", caseID, err)
		}
	}
	if _, err := RunInitialOracleGauntlets(context.Background(), request); err == nil {
		t.Fatal("oracle suite overwrote a completed run")
	}
}

func TestInitialOracleSuiteRequiresSeparateEvidenceAndEvaluationRoots(t *testing.T) {
	request := oracleSuiteTestRequest(t)
	request.EvaluationRoot = request.EpisodeRoot
	if _, err := RunInitialOracleGauntlets(context.Background(), request); err == nil {
		t.Fatal("oracle suite mixed episode evidence with private evaluations")
	}
}

func oracleSuiteTestRequest(t *testing.T) OracleSuiteRequest {
	t.Helper()
	root := t.TempDir()
	episodeRoot := filepath.Join(root, "episodes")
	evaluationRoot := filepath.Join(root, "evaluations")
	if err := os.Mkdir(episodeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(evaluationRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	return OracleSuiteRequest{
		Surface: SurfaceSymbolic, RatGeneration: mustRatGeneration(t),
		RuntimeFingerprint: transferTestFingerprint(), Repetition: 1,
		Actor: cognition.AttemptRef{
			JobID: 101, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "oracle-suite",
		},
		EpisodeRoot: episodeRoot, EvaluationRoot: evaluationRoot,
		LedgerSchemaVersion:     "task-ledger.v1",
		WorkingSetPolicyVersion: "working-set.v1",
		ProjectionPolicyVersion: "context-projection.v1",
	}
}
