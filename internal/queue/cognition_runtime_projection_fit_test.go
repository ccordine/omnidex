package queue

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestCognitionProjectionFitUsesRemainingBudgetForHighestPriorityOptionalContext(t *testing.T) {
	input := cognitionProjectionFitTestInput(t)
	oneSpec := cloneCognitionFitSpec(input.Spec)
	oneSpec.Optional[0].MaxItems = 1
	one, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: input.WorkID, Spec: oneSpec, WorkingSet: input.Set, Materials: input.Materials,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, oneEnvelope, err := measureCognitionProjection(input, one)
	if err != nil {
		t.Fatal(err)
	}
	_, fullEnvelope, err := measureCognitionProjection(input, input.Initial)
	if err != nil {
		t.Fatal(err)
	}
	if fullEnvelope.Bytes <= oneEnvelope.Bytes {
		t.Fatalf("full envelope=%d one optional=%d", fullEnvelope.Bytes, oneEnvelope.Bytes)
	}
	input.Budget.MaxInputBytes = cognitionModelVisibleInputBytes(oneEnvelope)
	reserve := input.Episode.AttestedBrain.Ref.Sampling.InputSpecialTokenReserve
	input.Budget.MaxInputTokens = cognitionModelInputTokenUpperBound(fullEnvelope, reserve) + 1

	fit, err := fitCognitionPolicyProjection(input)
	if err != nil {
		t.Fatal(err)
	}
	if cognitionModelVisibleInputBytes(fit.Envelope) > input.Budget.MaxInputBytes ||
		cognitionModelInputTokenUpperBound(fit.Envelope, reserve) > input.Budget.MaxInputTokens {
		t.Fatalf("fit envelope=%d/%d budget=%d/%d", fit.Envelope.Bytes,
			fit.Envelope.EstimatedTokens, input.Budget.MaxInputBytes, input.Budget.MaxInputTokens)
	}
	if len(fit.Projection.Selected) != 2 ||
		fit.Projection.Selected[0].ItemID != "required-goal" ||
		fit.Projection.Selected[1].ItemID != "optional-high" {
		t.Fatalf("fit selected=%#v", fit.Projection.Selected)
	}
	modelEvidence := fit.Snapshot.EvidenceRefs()
	trimmed := input.CompletionEvidence[1]
	if len(modelEvidence) != 1 || modelEvidence[0] != input.CompletionEvidence[0] ||
		strings.Contains(fit.Envelope.JSON, string(trimmed.ObservationID)) {
		t.Fatalf("trimmed evidence leaked into model authority: refs=%#v envelope=%s", modelEvidence, fit.Envelope.JSON)
	}
	graph, err := cognition.NewObligationGraph(2, input.Current.ID, []cognition.ObligationSpec{{
		ID: input.Current.ID, Desired: input.Current.Desired,
		DependsOn: []cognition.ObligationID{}, SupportingRefs: input.Current.SupportingRefs,
		CompletionCheck: input.Current.CompletionCheck,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(2); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(input.Current.ID, 2, cognition.ObligationActive); err != nil {
		t.Fatal(err)
	}
	action, err := cognition.NewActionRequest("inspect", []cognition.ActionArgument{})
	if err != nil {
		t.Fatal(err)
	}
	decision := cognition.CognitionDecision{
		ObligationID: input.Current.ID, Action: action,
		EvidenceRefs: []cognition.EvidenceRef{trimmed}, ExpectedEffect: "Inspect exact state.",
	}
	prepared := cognitionruntime.PreparedSnapshot{
		Snapshot: fit.Snapshot, ObligationGraph: graph.Snapshot(), GraphVersion: 1,
		CompletionEvidenceRefs: append([]cognition.EvidenceRef{}, input.CompletionEvidence...),
	}
	_, err = cognitionruntime.NewAcceptedDecisionRecovery(
		cognitionruntime.Binding{
			Episode: cognition.EpisodeRef{ID: input.Episode.EpisodeID}, Attempt: input.Attempt,
		}, "call-trimmed-evidence", prepared, decision, input.Episode.ActionCatalog.Schemas[0], nil,
	)
	if !errors.Is(err, cognitionruntime.ErrInvalidJournalState) {
		t.Fatalf("trimmed evidence decision error=%v", err)
	}
}

func TestCognitionProjectionFitFailsWhenRequiredEnvelopeCannotFit(t *testing.T) {
	input := cognitionProjectionFitTestInput(t)
	requiredSpec := cloneCognitionFitSpec(input.Spec)
	requiredSpec.Optional = []contextbuilder.Selector{}
	required, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: input.WorkID, Spec: requiredSpec, WorkingSet: input.Set, Materials: input.Materials,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, requiredEnvelope, err := measureCognitionProjection(input, required)
	if err != nil {
		t.Fatal(err)
	}
	input.Budget.MaxInputBytes = cognitionModelVisibleInputBytes(requiredEnvelope) - 1
	reserve := input.Episode.AttestedBrain.Ref.Sampling.InputSpecialTokenReserve
	input.Budget.MaxInputTokens = cognitionModelInputTokenUpperBound(requiredEnvelope, reserve) + 1024

	_, err = fitCognitionPolicyProjection(input)
	if !errors.Is(err, ErrCognitionEnvelopeBudget) {
		t.Fatalf("required overflow error=%v", err)
	}
}

func TestCognitionEnvelopeFitBindsExactPromptAtInputBoundary(t *testing.T) {
	t.Parallel()
	input := cognitionProjectionFitTestInput(t)
	_, envelope, err := measureCognitionProjection(input, input.Initial)
	if err != nil {
		t.Fatal(err)
	}
	budget := input.Budget
	budget.MaxInputBytes = cognitionModelVisibleInputBytes(envelope)
	reserve := input.Episode.AttestedBrain.Ref.Sampling.InputSpecialTokenReserve
	budget.MaxInputTokens = cognitionModelInputTokenUpperBound(envelope, reserve)
	if !cognitionEnvelopeFits(envelope, budget, reserve) {
		t.Fatal("exact model-visible input boundary was rejected")
	}
	budget.MaxInputBytes--
	if cognitionEnvelopeFits(envelope, budget, reserve) {
		t.Fatal("prompt hint bytes escaped the model-visible input ceiling")
	}
}

func cognitionProjectionFitTestInput(t *testing.T) cognitionProjectionFitInput {
	t.Helper()
	episodeID := cognition.EpisodeID("episode-projection-fit")
	revision, err := cognition.NewWorldRevision(episodeID, 1, strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	highContent, lowContent := strings.Repeat("h", 2048), strings.Repeat("l", 2048)
	highObservation, err := cognition.NewObservation("observation-fit-high", revision, "state", highContent)
	if err != nil {
		t.Fatal(err)
	}
	lowObservation, err := cognition.NewObservation("observation-fit-low", revision, "state", lowContent)
	if err != nil {
		t.Fatal(err)
	}
	set, err := workingset.New(workingset.Owner{
		LedgerID: taskstate.LedgerID("ledger_" + strings.Repeat("a", 64)), JobID: 71, Generation: 2,
	}, workingset.Budget{MaxItems: 8, MaxBytes: 32 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	required := acquireCognitionFitItem(t, set, "required-goal", workingset.RoleGoal, 100, taskstate.Ref{
		URI: "context:projection/goal", Version: "v1",
		Hash: strings.Repeat("d", 64), Relation: taskstate.RefEvidence,
	}, 256)
	high := acquireCognitionFitItem(
		t, set, "optional-high", workingset.RoleEvidence, 90,
		cognitionFitEvidenceTaskRef(highObservation.EvidenceRef()), len(highContent),
	)
	low := acquireCognitionFitItem(
		t, set, "optional-low", workingset.RoleEvidence, 80,
		cognitionFitEvidenceTaskRef(lowObservation.EvidenceRef()), len(lowContent),
	)
	materials := []contextbuilder.Material{
		cognitionFitMaterial(required, taskstate.AuthorityCode, strings.Repeat("g", 256)),
		cognitionFitMaterial(high, taskstate.AuthorityToolEvidence, highContent),
		cognitionFitMaterial(low, taskstate.AuthorityToolEvidence, lowContent),
	}
	predicate, err := cognition.NewPredicate("target.ready", []string{"target"})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	check := cognition.CompletionCheckRef{
		ID: "check.projection.fit", Version: "v1", SHA256: strings.Repeat("b", 64),
	}
	graph, err := cognition.NewObligationGraph(2, "obligation-projection-fit", []cognition.ObligationSpec{{
		ID: "obligation-projection-fit", Desired: goal, DependsOn: []cognition.ObligationID{},
		SupportingRefs: []cognition.EvidenceRef{}, CompletionCheck: check,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(2); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition("obligation-projection-fit", 2, cognition.ObligationActive); err != nil {
		t.Fatal(err)
	}
	current, _ := graph.Obligation("obligation-projection-fit")
	schema, err := cognition.NewActionSchema("schema.fit", "v1", "inspect", nil, cognition.EvidenceOptional)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := cognition.NewActionCatalog("catalog.fit", "v1", []cognition.ActionSchema{schema})
	if err != nil {
		t.Fatal(err)
	}
	goalRef := required.Item.Ref
	goalRef.Relation = taskstate.RefConcerns
	spec := contextbuilder.ContextSpec{
		Name: "cognition-fit", Version: "v1", ScopeRef: goalRef,
		Required: []contextbuilder.Selector{{
			ID: "goal", Role: workingset.RoleGoal, MinItems: 1, MaxItems: 1,
		}},
		Optional: []contextbuilder.Selector{{
			ID: "evidence", Role: workingset.RoleEvidence, MaxItems: 2,
		}},
		AllowedAuthorities: []taskstate.Authority{taskstate.AuthorityCode, taskstate.AuthorityToolEvidence},
		MaxItems:           3, MaxBytes: 16 * 1024,
	}
	initial, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: "projection-fit", Spec: spec, WorkingSet: set, Materials: materials,
	})
	if err != nil {
		t.Fatal(err)
	}
	return cognitionProjectionFitInput{
		Episode: CognitionEpisode{EpisodeID: episodeID, Goal: goal, CurrentRevision: revision, ActionCatalog: catalog},
		Current: current,
		Attempt: cognition.AttemptRef{
			JobID: 71, Generation: 2, StepID: 9, Attempt: 1, WorkerID: "worker-fit",
		},
		Budget: cognitionTestRuntimeBudget(), Set: set, WorkID: "projection-fit",
		Spec: spec, Materials: materials, Initial: initial,
		CompletionEvidence: []cognition.EvidenceRef{
			highObservation.EvidenceRef(), lowObservation.EvidenceRef(),
		},
	}
}

func acquireCognitionFitItem(
	t *testing.T,
	set *workingset.Set,
	id string,
	role workingset.Role,
	priority int,
	ref taskstate.Ref,
	bytes int,
) workingset.AcquireResult {
	t.Helper()
	result, err := set.Acquire(workingset.AcquireRequest{
		ID: workingset.ItemID(id), Ref: ref,
		Role: role, Retention: workingset.RetentionJob, Scope: set.Scope(), Priority: priority, ByteCost: bytes,
		Acquisition: workingset.Acquisition{
			Provider: workingset.ProviderTaskState, OperationID: "acquire-" + id,
			Reason: "Provide exact projection-fit test material.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func cognitionFitEvidenceTaskRef(ref cognition.EvidenceRef) taskstate.Ref {
	return taskstate.Ref{
		URI: cognitionEvidenceTaskRef(ref), Version: "1", Hash: ref.SHA256,
		Relation: taskstate.RefEvidence,
	}
}

func cognitionFitMaterial(
	item workingset.AcquireResult,
	authority taskstate.Authority,
	content string,
) contextbuilder.Material {
	return contextbuilder.Material{
		ItemID: item.Item.ID, CurrentRef: item.Item.Ref, SourceRefs: []taskstate.Ref{},
		Authority: authority, Content: content, ByteCost: len(content),
	}
}

func cloneCognitionFitSpec(spec contextbuilder.ContextSpec) contextbuilder.ContextSpec {
	spec.Required = append([]contextbuilder.Selector{}, spec.Required...)
	spec.Optional = append([]contextbuilder.Selector{}, spec.Optional...)
	spec.AllowedAuthorities = append([]taskstate.Authority{}, spec.AllowedAuthorities...)
	return spec
}
