package queue

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/exactjson"
)

func loadBudgetParitySnapshot(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	ordinal uint64,
) CognitionRuntimeSnapshotRecord {
	t.Helper()
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.Context)
	episode, found, err := loadCognitionEpisodeTx(fixture.Context, tx, fixture.EpisodeID, false)
	if err != nil || !found {
		t.Fatalf("load episode: found=%v error=%v", found, err)
	}
	graph, found, err := loadCurrentCognitionObligationGraphTx(
		fixture.Context, tx, fixture.EpisodeID, false,
	)
	if err != nil || !found {
		t.Fatalf("load graph: found=%v error=%v", found, err)
	}
	current, err := oneActiveCognitionObligation(graph.Graph)
	if err != nil {
		t.Fatal(err)
	}
	budget := episode.Budget
	budget.RemainingPolicyCalls -= uint32(ordinal - 1)
	record, found, err := loadCognitionSnapshotReplayTx(
		fixture.Context, tx, fixture.Authority, episode, graph, current.ID, ordinal, budget,
	)
	if err != nil || !found {
		t.Fatalf("load snapshot ordinal %d: found=%v error=%v", ordinal, found, err)
	}
	return record
}

func cloneBudgetSnapshot(
	fixture taskGenerationRetirementFixture,
	source CognitionRuntimeSnapshotRecord,
	remaining int64,
	ordinal int64,
	label string,
	changeMaximum bool,
) error {
	raw, sha := budgetParityBudgetJSON(source.Prepared.Snapshot.Budget(), remaining, changeMaximum)
	snapshotSHA := policyInputTestSHA(string(source.Prepared.Snapshot.SHA256()) + "\x00" + label)
	_, err := fixture.Pool.Exec(fixture.Context, `
		INSERT INTO cognition_runtime_snapshots (
			snapshot_sha256,preparation_id,episode_id,job_id,generation,step_id,
			actor_attempt,actor_worker_id,call_ordinal,expected_revision,
			expected_revision_sha256,obligation_node_id,graph_version,graph_sha256,
			projection_id,working_set_id,runtime_budget_json,runtime_budget_sha256,
			evidence_refs_json,evidence_refs_sha256,completion_evidence_refs_json,
			completion_evidence_refs_sha256,environment_terminal,public_outcome,
			policy_envelope_renderer_version,policy_envelope_token_estimator,
			policy_envelope_estimated_tokens,policy_envelope_sha256,policy_envelope_bytes,
			policy_prompt_hint_sha256,policy_prompt_hint_bytes,
			policy_model_visible_input_sha256,policy_model_visible_input_bytes,
			policy_model_visible_estimated_tokens,policy_model_input_token_upper_bound,
			policy_response_contract_sha256,policy_expected_provider_request_sha256
		)
		SELECT $1,'cognition_snapshot_'||$1,episode_id,job_id,generation,step_id,
		       actor_attempt,actor_worker_id,$2,expected_revision,expected_revision_sha256,
		       obligation_node_id,graph_version,graph_sha256,projection_id,working_set_id,
		       $3,$4,evidence_refs_json,evidence_refs_sha256,completion_evidence_refs_json,
		       completion_evidence_refs_sha256,environment_terminal,public_outcome,
		       policy_envelope_renderer_version,policy_envelope_token_estimator,
		       policy_envelope_estimated_tokens,policy_envelope_sha256,policy_envelope_bytes,
		       policy_prompt_hint_sha256,policy_prompt_hint_bytes,
		       policy_model_visible_input_sha256,policy_model_visible_input_bytes,
		       policy_model_visible_estimated_tokens,policy_model_input_token_upper_bound,
		       policy_response_contract_sha256,policy_expected_provider_request_sha256
		FROM cognition_runtime_snapshots WHERE snapshot_sha256=$5
	`, snapshotSHA, ordinal, string(raw), sha, source.Prepared.Snapshot.SHA256())
	return err
}

func budgetParityBudgetJSON(
	budget cognition.RuntimeBudget,
	remaining int64,
	changeMaximum bool,
) ([]byte, string) {
	budget.RemainingPolicyCalls = uint32(remaining)
	if changeMaximum {
		budget.MaxEvidenceRefs--
	}
	raw, sha, err := cognitionJSON(budget)
	if err != nil {
		panic(err)
	}
	return raw, sha
}

func nextBudgetParitySnapshot(
	source CognitionRuntimeSnapshotRecord,
) (CognitionRuntimeSnapshotRecord, error) {
	snapshot := source.Prepared.Snapshot
	budget := snapshot.Budget()
	budget.RemainingPolicyCalls--
	next, err := cognition.NewRuntimeSnapshot(
		snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
		snapshot.ActionCatalog(), snapshot.Attempt(), snapshot.ContextProjection(),
		budget, snapshot.EvidenceRefs(),
	)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	prepared := source.Prepared
	prepared.Snapshot = next
	if err := prepared.ValidateFor(cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: next.CurrentRevision().EpisodeID},
		Attempt: next.Attempt(),
	}); err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	return CognitionRuntimeSnapshotRecord{Prepared: prepared, CallOrdinal: source.CallOrdinal + 1}, nil
}

func budgetParityAttempt(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	snapshot CognitionRuntimeSnapshotRecord,
) cognitionpolicy.CallAttempt {
	t.Helper()
	projection, err := fixture.Repository.GetContextProjection(
		fixture.Context, string(snapshot.Prepared.Snapshot.ContextProjection().ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := buildBudgetParityAttempt(fixture, snapshot, projection.Projection)
	if err != nil {
		t.Fatal(err)
	}
	return attempt
}

func buildBudgetParityAttempt(
	fixture taskGenerationRetirementFixture,
	snapshot CognitionRuntimeSnapshotRecord,
	projection contextbuilder.Projection,
) (cognitionpolicy.CallAttempt, error) {
	journal := &captureCognitionCallJournal{}
	activation, err := fixture.Start.ProviderProcessActivation.Authority()
	if err != nil {
		return cognitionpolicy.CallAttempt{}, err
	}
	policy, err := cognitionpolicy.New(
		cognitionGuardPolicyClient{response: "unused"}, cognitionTestBrain(), activation,
		fixedBudgetParityProjection{projection: projection}, journal,
	)
	if err != nil {
		return cognitionpolicy.CallAttempt{}, err
	}
	if _, err := policy.Decide(context.Background(), snapshot.Prepared.Snapshot); err == nil {
		return cognitionpolicy.CallAttempt{}, context.Canceled
	}
	if journal.attempt.ID == "" {
		return cognitionpolicy.CallAttempt{}, context.Canceled
	}
	return journal.attempt, nil
}

func refreshForgedPolicyAttemptID(attempt *cognitionpolicy.CallAttempt) {
	raw, err := json.Marshal(attempt)
	if err != nil {
		panic(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	identity := map[string]any{}
	if err := decoder.Decode(&identity); err != nil {
		panic(err)
	}
	delete(identity, "id")
	canonical, err := exactjson.Canonical(identity)
	if err != nil {
		panic(err)
	}
	attempt.ID = "cognition_call_" + policyInputTestSHA(string(canonical))
}

func assertBudgetCallInsertRejected(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	attempt cognitionpolicy.CallAttempt,
) {
	t.Helper()
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.Context)
	authority, err := cognitionPolicyCallAuthority(attempt)
	if err != nil {
		t.Fatal(err)
	}
	if err := insertCognitionPolicyCallTx(fixture.Context, tx, authority, attempt); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(
		fixture.Context,
		`SET CONSTRAINTS cognition_policy_calls_snapshot_budget_exact IMMEDIATE`,
	)
	if err == nil || !strings.Contains(err.Error(), "exact durable call count") {
		t.Fatalf("stale orphan call error=%v", err)
	}
}

type fixedBudgetParityProjection struct{ projection contextbuilder.Projection }

func (value fixedBudgetParityProjection) LoadProjection(
	context.Context,
	cognition.ContextProjectionRef,
) (contextbuilder.Projection, error) {
	return value.projection, nil
}

func assertBudgetParityCounts(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	calls int,
	snapshots int,
) {
	t.Helper()
	var gotCalls, gotSnapshots int
	if err := fixture.Pool.QueryRow(fixture.Context, `SELECT
		(SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_runtime_snapshots WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&gotCalls, &gotSnapshots); err != nil {
		t.Fatal(err)
	}
	if gotCalls != calls || gotSnapshots != snapshots {
		t.Fatalf(
			"durable calls/snapshots=%d/%d want %d/%d",
			gotCalls, gotSnapshots, calls, snapshots,
		)
	}
}
