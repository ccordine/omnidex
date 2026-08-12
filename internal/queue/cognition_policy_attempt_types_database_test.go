package queue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
)

func TestPostgresPolicyCallAttemptRejectsJSONScalarCoercion(t *testing.T) {
	mutations := map[string]func(map[string]any){
		"job id string": func(document map[string]any) {
			mutatePolicyAttemptActorMember(document, "job_id", func(value any) any {
				return fmt.Sprint(value)
			})
		},
		"attempt string": func(document map[string]any) {
			mutatePolicyAttemptActorMember(document, "attempt", func(value any) any {
				return fmt.Sprint(value)
			})
		},
		"job id null": func(document map[string]any) {
			mutatePolicyAttemptActorMember(document, "job_id", func(any) any { return nil })
		},
		"attempt null": func(document map[string]any) {
			mutatePolicyAttemptActorMember(document, "attempt", func(any) any { return nil })
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			repository, pool, ctx := policyInputFreshRepository(t)
			fixture := startTaskGenerationRetirementFixtureIn(
				t, repository, pool, ctx, "attempt-scalar-"+strings.ReplaceAll(name, " ", "-"),
			)
			attempt := capturePreparedCognitionCall(t, fixture)
			raw, callID := forgedPolicyAttemptJSON(t, attempt, mutate)
			if err := insertRawForgedPolicyAttempt(fixture, attempt, callID, raw); err == nil {
				t.Fatal("direct SQL coerced a string/null policy-attempt actor number")
			}
			assertNoPolicyCallForSnapshot(t, fixture, attempt.SnapshotSHA256)
		})
	}
}

func mutatePolicyAttemptActorMember(
	document map[string]any,
	name string,
	mutate func(any) any,
) {
	actor := document["actor"].(map[string]any)
	activation := document["provider_process_activation"].(map[string]any)
	activationActor := activation["actor"].(map[string]any)
	actor[name] = mutate(actor[name])
	activationActor[name] = actor[name]
}

func forgedPolicyAttemptJSON(
	t *testing.T,
	attempt cognitionpolicy.CallAttempt,
	mutate func(map[string]any),
) ([]byte, string) {
	t.Helper()
	original, err := exactjson.Canonical(attempt)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(original))
	decoder.UseNumber()
	document := map[string]any{}
	if err := decoder.Decode(&document); err != nil {
		t.Fatal(err)
	}
	mutate(document)
	delete(document, "id")
	identity, err := exactjson.Canonical(document)
	if err != nil {
		t.Fatal(err)
	}
	callID := "cognition_call_" + policyInputTestSHA(string(identity))
	document["id"] = callID
	raw, err := exactjson.Canonical(document)
	if err != nil {
		t.Fatal(err)
	}
	return raw, callID
}

func insertRawForgedPolicyAttempt(
	fixture taskGenerationRetirementFixture,
	attempt cognitionpolicy.CallAttempt,
	callID string,
	raw []byte,
) error {
	budgetJSON, budgetSHA, err := cognitionJSON(attempt.RuntimeBudget)
	if err != nil {
		return err
	}
	brainJSON, brainSHA, err := cognitionJSON(attempt.Brain)
	if err != nil {
		return err
	}
	_, err = fixture.Pool.Exec(fixture.Context, `
		INSERT INTO cognition_policy_calls (
			call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id,
			snapshot_sha256,projection_id,working_set_id,expected_revision,
			expected_revision_sha256,obligation_node_id,runtime_budget_json,runtime_budget_sha256,
			brain_json,brain_sha256,attempt_json,attempt_sha256,status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,'started')
	`, callID, attempt.ExpectedRevision.EpisodeID, fixture.Authority.JobID,
		fixture.Authority.Generation, fixture.Authority.StepID, fixture.Authority.Attempt,
		fixture.Authority.WorkerID, attempt.SnapshotSHA256, attempt.ContextProjection.ID,
		attempt.ContextProjection.WorkingSetID, int64(attempt.ExpectedRevision.Number),
		attempt.ExpectedRevision.SHA256, attempt.ObligationID, string(budgetJSON), budgetSHA,
		string(brainJSON), brainSHA, string(raw), policyInputTestSHA(string(raw)))
	return err
}
