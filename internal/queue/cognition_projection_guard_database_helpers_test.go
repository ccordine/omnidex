package queue

import (
	"context"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5"
)

type cognitionGuardPolicyClient struct{ response string }

type cognitionGuardProjectionLoader struct{ repository *Repository }

func (cognitionGuardPolicyClient) AttestProviderIdentity(
	_ context.Context,
	expected llm.ProviderIdentityExpectation,
) (llm.ProviderIdentityAttestation, error) {
	return llm.NewProviderIdentityAttestation(
		expected, "queue-test:/version", "queue-test:/installed", "queue-test:/runner",
	)
}

func (loader cognitionGuardProjectionLoader) LoadProjection(
	ctx context.Context,
	ref cognition.ContextProjectionRef,
) (contextbuilder.Projection, error) {
	if loader.repository == nil {
		return contextbuilder.Projection{}, fmt.Errorf("projection repository is unavailable")
	}
	record, err := loader.repository.GetContextProjection(ctx, string(ref.ID))
	if err != nil {
		return contextbuilder.Projection{}, err
	}
	projection := record.Projection
	want := cognition.ContextProjectionRef{
		ID: cognition.ContextProjectionID(projection.ID), SHA256: projection.RenderedSHA256,
		WorkingSetID:      cognition.WorkingSetID(projection.WorkingSetID),
		WorkingSetVersion: projection.WorkingSetVersion, RendererVersion: projection.RendererVersion,
	}
	if record.Authority.Mode != ContextProjectionModeLive || want != ref {
		return contextbuilder.Projection{}, fmt.Errorf("projection %q authority differs from exact live ref", ref.ID)
	}
	return projection, nil
}

func (client cognitionGuardPolicyClient) Generate(context.Context, string, string) (string, error) {
	return client.response, nil
}

func (cognitionGuardPolicyClient) PrepareContextModel(
	_ context.Context,
	model string,
	prompt string,
) (llm.PreparedModel, error) {
	return llm.PreparedModel{BaseModel: model, ContextModel: model, Prompt: prompt}, nil
}

func (client cognitionGuardPolicyClient) GeneratePrepared(context.Context, llm.PreparedModel) (string, error) {
	return client.response, nil
}

func (cognitionGuardPolicyClient) CleanupPreparedModel(llm.PreparedModel) {}

func (cognitionGuardPolicyClient) RequireExactPreparedContract() error { return nil }

func (cognitionGuardPolicyClient) ValidateExactPreparedContract(prepared llm.PreparedModel) error {
	if prepared.PromptHint != llm.MinimalGeneratePrompt || prepared.MaxOutputTokens <= 0 ||
		prepared.ContextTokens <= 0 || prepared.ResponseFormat != llm.ResponseFormatJSON ||
		len(prepared.ResponseSchema) == 0 || prepared.ThinkingEnabled ||
		prepared.Temperature == nil || *prepared.Temperature != 0 {
		return fmt.Errorf("prepared cognition contract is not exact")
	}
	return nil
}

func (cognitionGuardPolicyClient) Embedding(context.Context, string) ([]float64, error) {
	return nil, nil
}

func prepareCognitionGuardAction(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	label string,
) CognitionActionRecord {
	t.Helper()
	schema := fixture.Start.ActionCatalog.Schemas[0]
	request, err := cognition.NewActionRequest(schema.Kind, []cognition.ActionArgument{})
	if err != nil {
		t.Fatal(err)
	}
	decision := cognition.CognitionDecision{
		ObligationID: fixture.Start.Root.ID, Action: request,
		EvidenceRefs: []cognition.EvidenceRef{}, ExpectedEffect: "Expose bounded public state.",
	}
	return prepareCognitionGuardActionWithDecision(t, fixture, decision)
}

func prepareCognitionGuardActionWithDecision(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	decision cognition.CognitionDecision,
) CognitionActionRecord {
	t.Helper()
	schema := fixture.Start.ActionCatalog.Schemas[0]
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		fixture.Context, CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := prepared.Prepared.Snapshot
	response, _, err := cognitionJSON(decision)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := cognitionpolicy.New(
		cognitionGuardPolicyClient{response: string(response)},
		cognitionTestBrain(),
		cognitionGuardProjectionLoader{repository: fixture.Repository},
		CognitionPolicyCallJournal{Repository: fixture.Repository},
	)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := cognition.NewCompletionResult(
		fixture.Start.Root.ID, fixture.Start.Root.CompletionCheck,
		snapshot.CurrentRevision(), cognition.CompletionUnsatisfied, []cognition.EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := cognition.NewCoordinator(policy)
	if err != nil {
		t.Fatal(err)
	}
	step, err := coordinator.Step(
		fixture.Context, snapshot, completion, prepared.Prepared.CompletionEvidenceRefs,
	)
	if err != nil {
		t.Fatal(err)
	}
	if step.Decision == nil {
		t.Fatal("coordinator produced no decision")
	}
	reconcile := cognitionruntime.ReconciliationCommand{
		Binding: cognitionruntime.Binding{
			Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: snapshot.Attempt(),
		},
		SnapshotSHA256: snapshot.SHA256(), Projection: snapshot.ContextProjection(),
		ActionSchema: schema, Decision: step.Decision.Clone(),
	}
	receipt, err := fixture.Repository.ReconcileCognitionRuntimeDecision(fixture.Context, reconcile)
	if err != nil {
		t.Fatal(err)
	}
	action, err := fixture.Repository.PrepareCognitionAction(fixture.Context, cognitionruntime.PrepareActionCommand{
		Binding: reconcile.Binding, Coordinator: step, Reconciliation: receipt,
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatched, err := fixture.Repository.DispatchCognitionAction(
		fixture.Context, fixture.Authority, action.Action.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return dispatched
}

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
	brainJSON, brainSHA, err := cognitionJSON(fixture.Start.AttestedBrain)
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
