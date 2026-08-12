package queue

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

func insertCognitionEpisodeWithoutStartTransition(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	tx pgx.Tx,
) {
	t.Helper()
	header, err := loadTaskLedgerHeaderTx(fixture.Context, tx, fixture.Job.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := createCognitionRootObligationTx(fixture.Context, tx, header, fixture.Start); err != nil {
		t.Fatal(err)
	}
	goalJSON, goalSHA, err := cognitionJSON(fixture.Start.Goal)
	if err != nil {
		t.Fatal(err)
	}
	catalogJSON, _, err := cognitionJSON(fixture.Start.ActionCatalog)
	if err != nil {
		t.Fatal(err)
	}
	budgetJSON, budgetSHA, err := cognitionJSON(fixture.Start.Budget)
	if err != nil {
		t.Fatal(err)
	}
	completionJSON, completionSHA, err := cognitionJSON(fixture.Start.Completion)
	if err != nil {
		t.Fatal(err)
	}
	brainJSON, brainSHA, err := cognitionJSON(fixture.Start.BrainBootstrap.AttestedBrain)
	if err != nil {
		t.Fatal(err)
	}
	factAuthority := cognitionTestFactAuthority().Reference()
	factJSON, factSHA, err := cognitionJSON(factAuthority)
	if err != nil {
		t.Fatal(err)
	}
	factIdentityJSON, factIdentitySHA, err := cognitionJSON(cognitionFactAuthorityIdentity(factAuthority))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fixture.Context, `
		INSERT INTO cognition_episodes (
			episode_id,schema_name,job_id,generation,step_id,created_attempt,created_worker_id,
			ledger_id,working_set_id,scenario_id,scenario_sha256,goal_json,goal_sha256,
			completion_authority_json,completion_authority_sha256,
			action_catalog_json,action_catalog_id,action_catalog_version,action_catalog_sha256,
			runtime_budget_json,runtime_budget_sha256,attested_brain_json,attested_brain_sha256,
			fact_authority_json,fact_authority_sha256,
			fact_authority_identity_json,fact_authority_identity_sha256,
			current_revision,current_revision_sha256,status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,'active')
	`, fixture.EpisodeID, cognitionEpisodeSchemaV1, fixture.Authority.JobID,
		fixture.Authority.Generation, fixture.Authority.StepID, fixture.Authority.Attempt,
		fixture.Authority.WorkerID, header.ID, fixture.WorkingSet,
		fixture.Start.Scenario.ID, fixture.Start.Scenario.SHA256, string(goalJSON), goalSHA,
		string(completionJSON), completionSHA, string(catalogJSON),
		fixture.Start.ActionCatalog.ID, fixture.Start.ActionCatalog.Version,
		fixture.Start.ActionCatalog.SHA256, string(budgetJSON), budgetSHA, string(brainJSON), brainSHA,
		string(factJSON), factSHA, string(factIdentityJSON), factIdentitySHA,
		int64(fixture.Start.Transition.Current.Number),
		fixture.Start.Transition.Current.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := insertCognitionObligationProjectionTx(
		fixture.Context, tx, fixture.Start, header.ID,
	); err != nil {
		t.Fatal(err)
	}
	graph, descriptor, err := initialCognitionObligationGraph(fixture.Start)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := insertCognitionObligationGraphTx(
		fixture.Context, tx, fixture.EpisodeID, 1, descriptor, graph, fixture.Authority,
	); err != nil {
		t.Fatal(err)
	}
}
