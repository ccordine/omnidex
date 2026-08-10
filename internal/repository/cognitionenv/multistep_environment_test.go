package cognitionenv

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestEnvironmentRestoresAndCompletesBoundedSearchInspectReferenceTraversal(t *testing.T) {
	investigation, analysis, snapshot := testInvestigation(
		t, repositoryretrieval.OperationDirectReferences,
	)
	builder := testRequestBuilder(t, analysis, snapshot)
	environment, episode, actor := testEnvironment(t, investigation, builder)
	start, err := environment.Start(t.Context(), investigation.Ref())
	if err != nil {
		t.Fatal(err)
	}
	search := testAction(t, investigation, actor, start.Observations[0].EvidenceRef())
	searched, err := environment.Apply(t.Context(), episode, start.Current, search)
	if err != nil {
		t.Fatal(err)
	}
	if searched.Terminal || searched.Current.Number != 2 {
		t.Fatalf("search transition=%+v", searched)
	}
	assertNoRepositoryPaths(t, searched, snapshot)

	target, found := testRequestSymbol(analysis, "Target")
	if !found {
		t.Fatal("target symbol is absent")
	}
	restored, err := NewEnvironment(
		investigation, episode, builder, environment.authorize, environment.journal,
	)
	if err != nil {
		t.Fatal(err)
	}
	inspect := testSymbolAction(
		t, investigation, actor, ActionInspect, target.ID,
		searched.Observations[1].EvidenceRef(),
	)
	inspected, err := restored.Apply(t.Context(), episode, searched.Current, inspect)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Terminal || inspected.Current.Number != 3 {
		t.Fatalf("inspect transition=%+v", inspected)
	}
	assertNoRepositoryPaths(t, inspected, snapshot)

	references := testSymbolAction(
		t, investigation, actor, ActionReferences, target.ID,
		inspected.Observations[1].EvidenceRef(),
	)
	completed, err := restored.Apply(t.Context(), episode, inspected.Current, references)
	if err != nil {
		t.Fatal(err)
	}
	if !completed.Terminal || completed.Current.Number != 4 ||
		completed.PublicOutcome != PublicOutcomeEvidenceAcquired {
		t.Fatalf("reference transition=%+v", completed)
	}
	assertNoRepositoryPaths(t, completed, snapshot)
	if got := requestOperations(builder.requests); !reflect.DeepEqual(got, []repositoryretrieval.Operation{
		repositoryretrieval.OperationSemanticExcerpts,
		repositoryretrieval.OperationSymbolDeclaration,
		repositoryretrieval.OperationDirectReferences,
	}) {
		t.Fatalf("retrieval operations=%v", got)
	}
}

func TestEnvironmentRejectsUndiscoveredAndUninspectedTraversalTargets(t *testing.T) {
	investigation, analysis, snapshot := testInvestigation(
		t, repositoryretrieval.OperationDirectReferences,
	)
	builder := testRequestBuilder(t, analysis, snapshot)
	environment, episode, actor := testEnvironment(t, investigation, builder)
	start, err := environment.Start(t.Context(), investigation.Ref())
	if err != nil {
		t.Fatal(err)
	}
	search := testAction(t, investigation, actor, start.Observations[0].EvidenceRef())
	searched, err := environment.Apply(t.Context(), episode, start.Current, search)
	if err != nil {
		t.Fatal(err)
	}
	target, _ := testRequestSymbol(analysis, "Target")
	references := testSymbolAction(
		t, investigation, actor, ActionReferences, target.ID,
		searched.Observations[1].EvidenceRef(),
	)
	_, err = environment.Apply(t.Context(), episode, searched.Current, references)
	var failure cognition.ActionFailure
	if !errors.As(err, &failure) || failure.Code != cognition.ActionFailurePreconditionFailed {
		t.Fatalf("uninspected reference error=%v", err)
	}
	if len(builder.requests) != 1 {
		t.Fatal("invalid traversal reached repository retrieval")
	}
}

func TestEnvironmentRejectsDiscoveredSubjectOutsideRegisteredQueryAuthority(t *testing.T) {
	investigation, analysis, snapshot := testInvestigation(
		t, repositoryretrieval.OperationDirectReferences,
	)
	caller, found := testRequestSymbol(analysis, "Caller")
	if !found {
		t.Fatal("caller symbol is absent")
	}
	builder := &recordingBuilder{build: func(
		request repositoryretrieval.Request,
	) (repositoryretrieval.EvidencePack, error) {
		pack, err := testPackForRequest(t, request, analysis, snapshot)
		if err != nil || request.Operation != repositoryretrieval.OperationSemanticExcerpts {
			return pack, err
		}
		pack.ID = ""
		pack.Symbols = append(pack.Symbols, testEvidenceSymbol(t, snapshot, caller))
		if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
			return repositoryretrieval.EvidencePack{}, err
		}
		return pack, nil
	}}
	environment, episode, actor := testEnvironment(t, investigation, builder)
	start, err := environment.Start(t.Context(), investigation.Ref())
	if err != nil {
		t.Fatal(err)
	}
	searched, err := environment.Apply(
		t.Context(), episode, start.Current,
		testAction(t, investigation, actor, start.Observations[0].EvidenceRef()),
	)
	if err != nil {
		t.Fatal(err)
	}
	wrong := testSymbolAction(
		t, investigation, actor, ActionInspect, caller.ID,
		searched.Observations[1].EvidenceRef(),
	)
	_, err = environment.Apply(t.Context(), episode, searched.Current, wrong)
	var failure cognition.ActionFailure
	if !errors.As(err, &failure) || failure.Code != cognition.ActionFailurePreconditionFailed {
		t.Fatalf("wrong discovered subject error=%v", err)
	}
	if len(builder.requests) != 1 {
		t.Fatal("wrong discovered subject reached bounded inspection retrieval")
	}
}

func requestOperations(requests []repositoryretrieval.Request) []repositoryretrieval.Operation {
	operations := make([]repositoryretrieval.Operation, len(requests))
	for index, request := range requests {
		operations[index] = request.Operation
	}
	return operations
}
