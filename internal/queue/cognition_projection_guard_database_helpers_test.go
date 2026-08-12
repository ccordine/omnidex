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
	"github.com/gryph/omnidex/internal/model"
)

type cognitionGuardPolicyClient struct{ response string }

type cognitionGuardProjectionLoader struct{ repository *Repository }

func cognitionGuardActivationAuthority(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
) cognitionpolicy.ProviderProcessActivationAuthority {
	t.Helper()
	authority, err := fixture.Start.ProviderProcessActivation.Authority()
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func cognitionGuardActivationAuthorityFor(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	episodeID cognition.EpisodeID,
	authority model.StepAttemptAuthority,
	brain cognitionpolicy.AttestedBrain,
) cognitionpolicy.ProviderProcessActivationAuthority {
	t.Helper()
	activation := cognitionGuardProviderProcessActivationFor(
		t, ctx, episodeID, authority, brain,
	)
	if err := repository.RecordCognitionProviderProcessObservation(
		ctx, activation,
	); err != nil {
		t.Fatal(err)
	}
	bound, err := activation.Authority()
	if err != nil {
		t.Fatal(err)
	}
	return bound
}

func cognitionGuardProviderProcessActivationFor(
	t *testing.T,
	ctx context.Context,
	episodeID cognition.EpisodeID,
	authority model.StepAttemptAuthority,
	brain cognitionpolicy.AttestedBrain,
) cognitionpolicy.ProviderProcessActivation {
	t.Helper()
	outcome, err := cognitionpolicy.ObserveProviderProcess(
		ctx, cognitionGuardPolicyClient{}, brain, cognition.EpisodeRef{ID: episodeID},
		cognition.AttemptRef{
			JobID: authority.JobID, Generation: authority.Generation,
			StepID: authority.StepID, Attempt: uint64(authority.Attempt),
			WorkerID: authority.WorkerID,
		}, cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if err != nil {
		t.Fatal(err)
	}
	activation, err := outcome.RequireSuccess(brain)
	if err != nil {
		t.Fatal(err)
	}
	return activation
}

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

func (cognitionGuardPolicyClient) ValidateExactPreparedProvider(
	expected llm.ProviderIdentityExpectation,
) error {
	return expected.Validate()
}

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
		cognitionGuardActivationAuthority(t, fixture),
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
