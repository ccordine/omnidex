package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func TestPostgresCognitionProposalMaterializationRejectsCallSnapshotForgery(t *testing.T) {
	fixture, materialization, unrelatedCallID := cognitionProposalCallSnapshotFixture(t)
	tx, err := fixture.Repository.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `
		ALTER TABLE cognition_reconciliations DISABLE TRIGGER USER
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		UPDATE cognition_reconciliations SET policy_call_id=$2
		WHERE reconciliation_id=$1
	`, materialization.ReconciliationID, unrelatedCallID); err != nil {
		t.Fatal(err)
	}
	installForgedCognitionProposalMaterializationTrigger(t, tx, materialization.ID)
	if _, err := tx.Exec(t.Context(), `
		UPDATE forged_cognition_proposal_materializations SET policy_call_id=$1
	`, unrelatedCallID); err != nil {
		t.Fatal(err)
	}
	expectForgedCognitionProposalMaterializationConstraint(t, tx)
}

func TestPostgresCognitionProposalMaterializationRejectsCallDecisionForgery(t *testing.T) {
	fixture, raw := cognitionProposalMaterializationFixture(t)
	materialization, err := DecodeCognitionProposalMaterialization(raw, cognitionPayloadSHA(raw))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := fixture.Repository.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `
		ALTER TABLE cognition_policy_calls DISABLE TRIGGER USER
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		WITH changed AS (
			SELECT cognition_canonical_jsonb(jsonb_set(
				result_json::jsonb,'{decision_sha256}',to_jsonb($2::TEXT),false
			)) AS body
			FROM cognition_policy_calls WHERE call_id=$1
		)
		UPDATE cognition_policy_calls calls
		SET result_json=changed.body,
			result_sha256=encode(digest(changed.body,'sha256'),'hex')
		FROM changed WHERE calls.call_id=$1
	`, materialization.PolicyCallID, cognitionTestDigest("7")); err != nil {
		t.Fatal(err)
	}
	installForgedCognitionProposalMaterializationTrigger(t, tx, materialization.ID)
	expectForgedCognitionProposalMaterializationConstraint(t, tx)
}

func cognitionProposalCallSnapshotFixture(
	t *testing.T,
) (cognitionDatabaseFixture, CognitionProposalMaterialization, string) {
	t.Helper()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "063")); err != nil {
		t.Fatal(err)
	}
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(
		t.Context(), fixture.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	firstDecision := cognitionProposalMaterializationDecision(fixture)
	action := prepareCognitionDecisionAction(t, fixture, firstDecision)
	next, err := cognition.NewWorldRevision(fixture.EpisodeID, 2, cognitionTestDigest("6"))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := cognition.NewActionObservation(
		"observation-proposal-call-snapshot", action.Action.ID, next,
		"public_state", "A second bounded public state is now visible.",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, action.Action.ID, cognition.Transition{
			ActionID: action.Action.ID, Previous: cognitionRevisionPointer(action.ExpectedRevision),
			Current: next, Observations: []cognition.Observation{observation},
			Effects: []cognition.Effect{}, Cost: 1,
		}, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	secondDecision := cognitionProposalMaterializationDecision(fixture)
	second := buildCognitionDecisionStep(t, fixture, secondDecision)
	var unrelatedCallID string
	if err := pool.QueryRow(t.Context(), `
		SELECT call_id FROM cognition_policy_calls WHERE snapshot_sha256=$1
	`, second.Command.SnapshotSHA256).Scan(&unrelatedCallID); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := pool.QueryRow(t.Context(), `
		SELECT payload_json FROM cognition_proposal_materializations
		WHERE episode_id=$1 ORDER BY proposal_index LIMIT 1
	`, fixture.EpisodeID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	materialization, err := DecodeCognitionProposalMaterialization(raw, cognitionPayloadSHA(raw))
	if err != nil {
		t.Fatal(err)
	}
	var unrelatedSnapshotSHA, unrelatedDecisionSHA string
	if err := pool.QueryRow(t.Context(), `
		SELECT snapshot_sha256,result_json::jsonb->>'decision_sha256'
		FROM cognition_policy_calls WHERE call_id=$1
	`, unrelatedCallID).Scan(&unrelatedSnapshotSHA, &unrelatedDecisionSHA); err != nil {
		t.Fatal(err)
	}
	if unrelatedSnapshotSHA == materialization.SnapshotSHA256 ||
		unrelatedDecisionSHA != materialization.DecisionSHA256 {
		t.Fatalf(
			"snapshot-only forgery tuple=%q/%q want different snapshot and decision %q",
			unrelatedSnapshotSHA, unrelatedDecisionSHA, materialization.DecisionSHA256,
		)
	}
	return fixture, materialization, unrelatedCallID
}

func installForgedCognitionProposalMaterializationTrigger(
	t *testing.T, tx pgx.Tx, materializationID string,
) {
	t.Helper()
	if _, err := tx.Exec(t.Context(), `
		CREATE TEMP TABLE forged_cognition_proposal_materializations
		(LIKE cognition_proposal_materializations INCLUDING DEFAULTS) ON COMMIT DROP
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO forged_cognition_proposal_materializations
		SELECT * FROM cognition_proposal_materializations WHERE materialization_id=$1
	`, materializationID); err != nil {
		t.Fatal(err)
	}
}

func expectForgedCognitionProposalMaterializationConstraint(t *testing.T, tx pgx.Tx) {
	t.Helper()
	if _, err := tx.Exec(t.Context(), `
		CREATE CONSTRAINT TRIGGER forged_cognition_proposal_materialization_exact
		AFTER INSERT ON forged_cognition_proposal_materializations
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
		EXECUTE FUNCTION require_exact_cognition_proposal_materialization()
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO forged_cognition_proposal_materializations
		SELECT * FROM forged_cognition_proposal_materializations LIMIT 1
	`); err != nil {
		t.Fatal(err)
	}
	_, err := tx.Exec(t.Context(), `
		SET CONSTRAINTS forged_cognition_proposal_materialization_exact IMMEDIATE
	`)
	if err == nil || !strings.Contains(err.Error(), "lacks exact reconciliation and ledger authority") {
		t.Fatalf("forged policy call authority constraint error=%v", err)
	}
}
