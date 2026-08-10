package cognitionpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

const policyTestDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type policyTestCallJournal struct {
	attempts         []CallAttempt
	results          []CallResult
	evidence         []ModelResponseEvidence
	providerEvidence []ProviderGenerationEvidence
	reservation      *CallReservation
	startErr         error
	finishErr        error
}

func (journal *policyTestCallJournal) Start(
	_ context.Context,
	attempt CallAttempt,
) (CallReservation, error) {
	journal.attempts = append(journal.attempts, attempt.Clone())
	if journal.startErr != nil {
		return CallReservation{}, journal.startErr
	}
	if journal.reservation != nil {
		return *journal.reservation, nil
	}
	return CallReservation{Attempt: attempt.Clone(), Created: true}, nil
}

func (journal *policyTestCallJournal) Finish(
	_ context.Context,
	attempt CallAttempt,
	result CallResult,
	evidence CallEvidence,
) error {
	journal.results = append(journal.results, result.Clone())
	journal.evidence = append(journal.evidence, evidence.Response.Clone())
	journal.providerEvidence = append(
		journal.providerEvidence, evidence.ProviderGeneration.Clone(),
	)
	return journal.finishErr
}

type policyTestProjectionLoader struct {
	projections map[cognition.ContextProjectionID]contextbuilder.Projection
	err         error
	calls       int
}

func newPolicyTestProjectionLoader(
	projection contextbuilder.Projection,
) *policyTestProjectionLoader {
	return &policyTestProjectionLoader{projections: map[cognition.ContextProjectionID]contextbuilder.Projection{
		cognition.ContextProjectionID(projection.ID): cloneProjection(projection),
	}}
}

func (loader *policyTestProjectionLoader) LoadProjection(
	_ context.Context,
	ref cognition.ContextProjectionRef,
) (contextbuilder.Projection, error) {
	loader.calls++
	if loader.err != nil {
		return contextbuilder.Projection{}, loader.err
	}
	projection, exists := loader.projections[ref.ID]
	if !exists {
		return contextbuilder.Projection{}, errors.New("projection not found")
	}
	return cloneProjection(projection), nil
}

func policyTestProjection(t *testing.T, content string) contextbuilder.Projection {
	t.Helper()
	ledgerID, err := taskstate.NewLedgerID(taskstate.LedgerOwner{
		Kind: taskstate.OwnerJob, JobID: 1,
		RunID: "01234567-89ab-cdef-0123-456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	byteBudget := len(content) + 8192
	set, err := workingset.New(
		workingset.Owner{LedgerID: ledgerID, JobID: 1, Generation: 1},
		workingset.Budget{MaxItems: 1, MaxBytes: byteBudget, MaxPinnedItems: 1, MaxPinnedBytes: byteBudget},
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := taskstate.Ref{
		URI: "task:job/1/entry/authority", Version: "v1",
		Hash: policyTestDigest, Relation: taskstate.RefSource,
	}
	acquired, err := set.Acquire(workingset.AcquireRequest{
		ID: "authority", Ref: ref, Role: workingset.RoleUserAuthority,
		Retention: workingset.RetentionJob, Scope: set.Scope(), Priority: 100,
		ByteCost: len(content), Acquisition: workingset.Acquisition{
			Provider: workingset.ProviderTaskState, OperationID: "operation-authority",
			Reason: "authoritative test material",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := contextbuilder.ContextSpec{
		Name: "cognition-policy", Version: "1.0.0",
		ScopeRef: taskstate.Ref{
			URI: "task:job/1", Version: "v1", Hash: policyTestDigest,
			Relation: taskstate.RefConcerns,
		},
		Required: []contextbuilder.Selector{{
			ID: "authority", Role: workingset.RoleUserAuthority, MinItems: 1, MaxItems: 1,
		}},
		AllowedAuthorities: []taskstate.Authority{taskstate.AuthorityUser},
		MaxItems:           1, MaxBytes: byteBudget, MaxAcquisitionRounds: 0,
	}
	projection, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: "work-1", Spec: spec, WorkingSet: set,
		Materials: []contextbuilder.Material{{
			ItemID: acquired.Item.ID, CurrentRef: acquired.Item.Ref,
			SourceRefs: []taskstate.Ref{acquired.Item.Ref},
			Authority:  taskstate.AuthorityUser, Content: content, ByteCost: len(content),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func policyTestSnapshot(t *testing.T, projection contextbuilder.Projection) (cognition.RuntimeSnapshot, cognition.EvidenceRef) {
	t.Helper()
	revision := cognition.WorldRevision{EpisodeID: "episode-1", Number: 1, SHA256: policyTestDigest}
	observation, err := cognition.NewObservation("observation-1", revision, "record", "public evidence")
	if err != nil {
		t.Fatal(err)
	}
	evidence := observation.EvidenceRef()
	predicate, err := cognition.NewPredicate("goal.condition", []string{"target-1"})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	check := cognition.CompletionCheckRef{ID: "check.goal", Version: "1.0.0", SHA256: policyTestDigest}
	obligation := cognition.Obligation{
		ID: "obligation-1", Desired: goal, Status: cognition.ObligationActive,
		SupportingRefs: []cognition.EvidenceRef{evidence}, CompletionCheck: check,
		CreatedGeneration: 1,
	}
	schema, err := cognition.NewActionSchema(
		"catalog.inspect.v1", "1.0.0", "inspect",
		[]cognition.ActionParameterSpec{{Name: "target", Required: true, MaxBytes: 128}},
		cognition.EvidenceRequired,
	)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := cognition.NewActionCatalog("catalog.policy", "1.0.0", []cognition.ActionSchema{schema})
	if err != nil {
		t.Fatal(err)
	}
	projectionRef := cognition.ContextProjectionRef{
		ID: cognition.ContextProjectionID(projection.ID), SHA256: projection.RenderedSHA256,
		WorkingSetID:      cognition.WorkingSetID(projection.WorkingSetID),
		WorkingSetVersion: projection.WorkingSetVersion, RendererVersion: projection.RendererVersion,
	}
	snapshot, err := cognition.NewRuntimeSnapshot(
		goal, revision, obligation, catalog,
		cognition.AttemptRef{JobID: 1, Generation: 1, StepID: 2, Attempt: 1, WorkerID: "worker-1"},
		projectionRef,
		cognition.RuntimeBudget{
			RemainingPolicyCalls: 1, MaxInputBytes: 64 * 1024, MaxInputTokens: 64*1024 + 2,
			MaxOutputBytes: 16 * 1024, MaxOutputTokens: 4 * 1024,
			MaxEvidenceRefs: 4, MaxActionArguments: 4,
			MaxLedgerProposals: 4, MaxAttentionRequests: 4, MaxExpectedEffectBytes: 512,
		},
		[]cognition.EvidenceRef{evidence},
	)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, evidence
}

func policyTestResponse(t *testing.T, snapshot cognition.RuntimeSnapshot, evidence cognition.EvidenceRef) string {
	t.Helper()
	raw, err := json.Marshal(cognition.CognitionDecision{
		ObligationID: snapshot.CurrentObligation().ID,
		Action: cognition.ActionRequest{Kind: "inspect", Arguments: []cognition.ActionArgument{{
			Name: "target", Value: "entity-1",
		}}},
		EvidenceRefs: []cognition.EvidenceRef{evidence}, ExpectedEffect: "Expose public properties.",
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
