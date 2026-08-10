package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresPolicyCallRequiresExactRenderedSnapshotInput(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "policy-exact-rendered-input",
	)
	exact := capturePreparedCognitionCall(t, fixture)
	mutations := map[string]func(*cognitionpolicy.CallAttempt){
		"envelope": func(value *cognitionpolicy.CallAttempt) {
			value.Envelope = strings.Replace(value.Envelope,
				"Select exactly one registered action", "Choose exactly one registered action", 1)
		},
		"response contract": func(value *cognitionpolicy.CallAttempt) {
			value.ResponseContractSHA256 = strings.Repeat("b", 64)
		},
		"prompt hint": func(value *cognitionpolicy.CallAttempt) {
			value.PromptHint = "Return a different caller-authored shape."
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run("journal rejects "+name, func(t *testing.T) {
			forged := exact
			mutate(&forged)
			refreshForgedPolicyAttempt(t, &forged)
			if _, err := fixture.Repository.StartCognitionPolicyCall(fixture.Context, forged); err == nil {
				t.Fatalf("journal accepted caller-authored %s", name)
			}
			assertNoPolicyCallForSnapshot(t, fixture, exact.SnapshotSHA256)
		})
		t.Run("database rejects "+name, func(t *testing.T) {
			forged := exact
			mutate(&forged)
			refreshForgedPolicyAttempt(t, &forged)
			if err := insertForgedPolicyAttempt(t, fixture, forged); err == nil {
				t.Fatalf("direct SQL accepted caller-authored %s", name)
			}
			assertNoPolicyCallForSnapshot(t, fixture, exact.SnapshotSHA256)
		})
	}

	reservation, err := fixture.Repository.StartCognitionPolicyCall(fixture.Context, exact)
	if err != nil {
		t.Fatalf("exact call after rejected forgeries: %v", err)
	}
	if !reservation.Created {
		t.Fatal("exact call was not newly reserved")
	}
}

func policyInputFreshRepository(t *testing.T) (*Repository, *pgxpool.Pool, context.Context) {
	t.Helper()
	pool := openIsolatedMigrationPool(t)
	directory := t.TempDir()
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !migrationFileNamePattern.MatchString(entry.Name()) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("../../migrations", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, entry.Name()), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository := New(pool)
	ctx := context.Background()
	if err := repository.EnsureSchema(ctx, loadMigrationTestBundle(t, directory)); err != nil {
		t.Fatal(err)
	}
	return repository, pool, ctx
}

func refreshForgedPolicyAttempt(t *testing.T, attempt *cognitionpolicy.CallAttempt) {
	t.Helper()
	attempt.EnvelopeBytes = len(attempt.Envelope)
	attempt.EnvelopeEstimatedTokens = (attempt.EnvelopeBytes + 3) / 4
	attempt.EnvelopeSHA256 = policyInputTestSHA(attempt.Envelope)
	attempt.PromptHintBytes = len(attempt.PromptHint)
	attempt.PromptHintSHA256 = policyInputTestSHA(attempt.PromptHint)
	modelInput := attempt.Envelope + llm.ExactPreparedPromptJoiner + attempt.PromptHint
	attempt.ModelVisibleInputBytes = len(modelInput)
	attempt.ModelVisibleEstimatedTokens = (attempt.ModelVisibleInputBytes + 3) / 4
	upper, err := llm.ModelInputTokenUpperBound(
		modelInput, attempt.Brain.Sampling.InputSpecialTokenReserve,
	)
	if err != nil {
		t.Fatal(err)
	}
	attempt.ModelInputTokenUpperBound = upper
	attempt.ModelVisibleInputSHA256 = policyInputTestSHA(modelInput)
	attempt.ID = forgedPolicyAttemptID(t, *attempt)
}

func forgedPolicyAttemptID(t *testing.T, attempt cognitionpolicy.CallAttempt) string {
	t.Helper()
	raw, err := json.Marshal(attempt)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var identity map[string]any
	if err := decoder.Decode(&identity); err != nil {
		t.Fatal(err)
	}
	delete(identity, "id")
	canonical, err := exactjson.Canonical(identity)
	if err != nil {
		t.Fatal(err)
	}
	return "cognition_call_" + policyInputTestSHA(string(canonical))
}

func policyInputTestSHA(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func insertForgedPolicyAttempt(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	attempt cognitionpolicy.CallAttempt,
) error {
	t.Helper()
	budgetJSON, budgetSHA, err := cognitionJSON(attempt.RuntimeBudget)
	if err != nil {
		t.Fatal(err)
	}
	brainJSON, brainSHA, err := cognitionJSON(attempt.Brain)
	if err != nil {
		t.Fatal(err)
	}
	attemptJSON, err := exactjson.Canonical(attempt)
	if err != nil {
		t.Fatal(err)
	}
	attemptSHA := policyInputTestSHA(string(attemptJSON))
	_, err = fixture.Pool.Exec(fixture.Context, `
		INSERT INTO cognition_policy_calls (
			call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id,
			snapshot_sha256,projection_id,working_set_id,expected_revision,
			expected_revision_sha256,obligation_node_id,runtime_budget_json,runtime_budget_sha256,
			brain_json,brain_sha256,attempt_json,attempt_sha256,status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,'started')
	`, attempt.ID, attempt.ExpectedRevision.EpisodeID, fixture.Authority.JobID,
		fixture.Authority.Generation, fixture.Authority.StepID, fixture.Authority.Attempt,
		fixture.Authority.WorkerID, attempt.SnapshotSHA256, attempt.ContextProjection.ID,
		attempt.ContextProjection.WorkingSetID, int64(attempt.ExpectedRevision.Number),
		attempt.ExpectedRevision.SHA256, attempt.ObligationID, string(budgetJSON), budgetSHA,
		string(brainJSON), brainSHA, string(attemptJSON), attemptSHA)
	return err
}

func assertNoPolicyCallForSnapshot(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	snapshotSHA string,
) {
	t.Helper()
	var count int
	if err := fixture.Pool.QueryRow(fixture.Context,
		`SELECT COUNT(*) FROM cognition_policy_calls WHERE snapshot_sha256=$1`, snapshotSHA,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejected input consumed policy budget: count=%d", count)
	}
}
