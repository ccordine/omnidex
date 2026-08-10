package cognitionenv

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestInvestigationRegistersOnlyReadActionsNeededByItsExactOperation(t *testing.T) {
	for _, test := range []struct {
		operation repositoryretrieval.Operation
		kinds     []cognition.ActionKind
	}{
		{repositoryretrieval.OperationSemanticExcerpts, []cognition.ActionKind{ActionSearch}},
		{repositoryretrieval.OperationSymbolDeclaration, []cognition.ActionKind{ActionInspect, ActionSearch}},
		{repositoryretrieval.OperationDirectReferences, []cognition.ActionKind{
			ActionInspect, ActionReferences, ActionSearch,
		}},
	} {
		t.Run(string(test.operation), func(t *testing.T) {
			investigation, _, _ := testInvestigation(t, test.operation)
			catalog := investigation.Catalog()
			if len(catalog.Schemas) != len(test.kinds) {
				t.Fatalf("catalog=%+v", catalog)
			}
			for index, schema := range catalog.Schemas {
				parameters := 1
				if schema.Kind == ActionSearch {
					parameters = 0
				}
				if schema.Kind != test.kinds[index] || len(schema.Parameters) != parameters ||
					schema.EvidencePolicy != cognition.EvidenceRequired {
					t.Fatalf("catalog schema=%+v", schema)
				}
			}
			if investigation.Completion().Check != catalogCompletionCheck(t, investigation) {
				t.Fatal("completion check is not bound to the exact investigation catalog")
			}
		})
	}
}

func TestEnvironmentProducesBoundedPathBlindEvidenceAndExactReplay(t *testing.T) {
	investigation, analysis, snapshot := testInvestigation(
		t, repositoryretrieval.OperationSemanticExcerpts,
	)
	builder := &recordingBuilder{pack: testPack(t, investigation, analysis, snapshot)}
	environment, episode, actor := testEnvironment(t, investigation, builder)
	start, err := environment.Start(t.Context(), investigation.Ref())
	if err != nil {
		t.Fatal(err)
	}
	assertNoRepositoryPaths(t, start, snapshot)
	action := testAction(t, investigation, actor, start.Observations[0].EvidenceRef())
	first, err := environment.Apply(t.Context(), episode, start.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	second, err := environment.Apply(t.Context(), episode, start.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("exact action replay returned another transition")
	}
	if !first.Terminal || first.PublicOutcome != PublicOutcomeEvidenceAcquired ||
		len(first.Observations) != 2 || first.Observations[0].Kind != ObservationEvidence ||
		first.Observations[1].Kind != ObservationState {
		t.Fatalf("transition=%+v", first)
	}
	assertNoRepositoryPaths(t, first, snapshot)
	if len(builder.requests) != 1 ||
		builder.requests[0].Operation != investigation.Operation() {
		t.Fatalf("requests=%+v", builder.requests)
	}

	changedReplay := action.Clone()
	changedReplay.EvidenceRefs = append(changedReplay.EvidenceRefs, cognition.EvidenceRef{
		ObservationID: "repository-observation-other", Revision: start.Current,
		SHA256: strings.Repeat("e", 64),
	})
	_, err = environment.Apply(t.Context(), episode, start.Current, changedReplay)
	var failure cognition.ActionFailure
	if !errors.As(err, &failure) || failure.Code != cognition.ActionFailureIdempotencyConflict {
		t.Fatalf("changed replay error=%v", err)
	}
	newAction := action.Clone()
	newAction.ID = "repository-action-after-terminal"
	_, err = environment.Apply(t.Context(), episode, first.Current, newAction)
	if !errors.As(err, &failure) || failure.Code != cognition.ActionFailureTerminal {
		t.Fatalf("terminal error=%v", err)
	}
}

func TestEnvironmentRejectsWholeFileEvidenceBeforeDurableCommit(t *testing.T) {
	investigation, analysis, snapshot := testInvestigation(
		t, repositoryretrieval.OperationSemanticExcerpts,
	)
	pack := testPack(t, investigation, analysis, snapshot)
	pack.ID = ""
	symbolFileSize := evidenceSymbolFileSize(t, pack.Symbols[0].ID, analysis, snapshot)
	pack.Symbols[0].Source = strings.Repeat("x", int(symbolFileSize))
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	builder := &recordingBuilder{pack: pack}
	environment, episode, actor := testEnvironment(t, investigation, builder)
	start, err := environment.Start(t.Context(), investigation.Ref())
	if err != nil {
		t.Fatal(err)
	}
	action := testAction(t, investigation, actor, start.Observations[0].EvidenceRef())
	if _, err := environment.Apply(t.Context(), episode, start.Current, action); err == nil ||
		!strings.Contains(err.Error(), "refuses to expose a whole file") {
		t.Fatalf("whole-file repository evidence error=%v", err)
	}
	state, err := environment.journal.EnvironmentState(t.Context(), episode, investigation.Ref())
	if err != nil || state.Current != start.Current || state.CurrentReceipt != nil {
		t.Fatalf("whole-file evidence mutated journal state=%+v error=%v", state, err)
	}
}

func TestConcurrentSameRevisionActionsCommitExactlyOneTerminalReceipt(t *testing.T) {
	investigation, analysis, snapshot := testInvestigation(
		t, repositoryretrieval.OperationSemanticExcerpts,
	)
	builder := &recordingBuilder{pack: testPack(t, investigation, analysis, snapshot)}
	environment, episode, actor := testEnvironment(t, investigation, builder)
	start, err := environment.Start(t.Context(), investigation.Ref())
	if err != nil {
		t.Fatal(err)
	}
	left := testAction(t, investigation, actor, start.Observations[0].EvidenceRef())
	right := left.Clone()
	right.ID = "repository-action-concurrent-right"
	type result struct {
		transition cognition.Transition
		err        error
	}
	results := make(chan result, 2)
	for _, action := range []cognition.RegisteredAction{left, right} {
		action := action
		go func() {
			transition, applyErr := environment.Apply(t.Context(), episode, start.Current, action)
			results <- result{transition: transition, err: applyErr}
		}()
	}
	var successes, terminalFailures int
	for range 2 {
		result := <-results
		if result.err == nil {
			if !result.transition.Terminal {
				t.Fatalf("successful transition is not terminal: %+v", result.transition)
			}
			successes++
			continue
		}
		var failure cognition.ActionFailure
		if errors.As(result.err, &failure) && failure.Code == cognition.ActionFailureTerminal {
			terminalFailures++
			continue
		}
		t.Fatalf("unexpected concurrent result: %v", result.err)
	}
	if successes != 1 || terminalFailures != 1 {
		t.Fatalf("successes=%d terminal_failures=%d", successes, terminalFailures)
	}
	state, err := environment.journal.EnvironmentState(t.Context(), episode, investigation.Ref())
	if err != nil || !state.Terminal || state.TerminalReceipt == nil {
		t.Fatalf("journal state=%+v error=%v", state, err)
	}
}

func TestEnvironmentRejectsActionWithoutExactNeedEvidence(t *testing.T) {
	investigation, analysis, snapshot := testInvestigation(
		t, repositoryretrieval.OperationSymbolDeclaration,
	)
	builder := &recordingBuilder{pack: testPack(t, investigation, analysis, snapshot)}
	environment, episode, actor := testEnvironment(t, investigation, builder)
	start, err := environment.Start(t.Context(), investigation.Ref())
	if err != nil {
		t.Fatal(err)
	}
	otherEvidence := cognition.EvidenceRef{
		ObservationID: "unrelated-observation", Revision: start.Current,
		SHA256: strings.Repeat("f", 64),
	}
	action := testAction(t, investigation, actor, otherEvidence)
	_, err = environment.Apply(t.Context(), episode, start.Current, action)
	var failure cognition.ActionFailure
	if !errors.As(err, &failure) || failure.Code != cognition.ActionFailurePreconditionFailed {
		t.Fatalf("missing evidence error=%v", err)
	}
	if len(builder.requests) != 0 {
		t.Fatal("evidence acquisition ran before its exact precondition")
	}
}

func TestCompletionIsCodeOwnedAndRequiresTerminalObservation(t *testing.T) {
	investigation, analysis, snapshot := testInvestigation(
		t, repositoryretrieval.OperationDirectReferences,
	)
	builder := testRequestBuilder(t, analysis, snapshot)
	environment, episode, actor := testEnvironment(t, investigation, builder)
	start, err := environment.Start(t.Context(), investigation.Ref())
	if err != nil {
		t.Fatal(err)
	}
	binding, err := cognitionruntime.NewBinding(episode, actor)
	if err != nil {
		t.Fatal(err)
	}
	obligation := activeObligation(t, investigation, actor.Generation)
	request := cognitionruntime.CompletionRequest{
		Binding: binding, SnapshotSHA256: strings.Repeat("a", 64), Goal: investigation.Goal(),
		Revision: start.Current, Obligation: obligation,
		EvidenceRefs: []cognition.EvidenceRef{start.Observations[0].EvidenceRef()},
	}
	unsatisfied, err := environment.Evaluate(t.Context(), request)
	if err != nil || unsatisfied.Outcome != cognition.CompletionUnsatisfied {
		t.Fatalf("unsatisfied=%+v error=%v", unsatisfied, err)
	}
	searchAction := testAction(t, investigation, actor, start.Observations[0].EvidenceRef())
	searched, err := environment.Apply(t.Context(), episode, start.Current, searchAction)
	if err != nil {
		t.Fatal(err)
	}
	if searched.Terminal || len(searched.Observations) != 2 {
		t.Fatalf("search transition=%+v", searched)
	}
	target, found := testRequestSymbol(analysis, "Target")
	if !found {
		t.Fatal("target symbol is absent")
	}
	inspectAction := testSymbolAction(
		t, investigation, actor, ActionInspect, target.ID,
		searched.Observations[1].EvidenceRef(),
	)
	inspected, err := environment.Apply(t.Context(), episode, searched.Current, inspectAction)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Terminal {
		t.Fatal("direct-reference investigation terminated before reference traversal")
	}
	referencesAction := testSymbolAction(
		t, investigation, actor, ActionReferences, target.ID,
		inspected.Observations[1].EvidenceRef(),
	)
	transition, err := environment.Apply(t.Context(), episode, inspected.Current, referencesAction)
	if err != nil {
		t.Fatal(err)
	}
	request.Revision = transition.Current
	request.EnvironmentTerminal = true
	request.PublicOutcome = transition.PublicOutcome
	if _, err := environment.Evaluate(t.Context(), request); err == nil ||
		!strings.Contains(err.Error(), "terminal repository evidence is absent") {
		t.Fatalf("missing terminal evidence error=%v", err)
	}
	request.EvidenceRefs = append(request.EvidenceRefs, transition.Observations[0].EvidenceRef())
	satisfied, err := environment.Evaluate(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if satisfied.Outcome != cognition.CompletionSatisfied ||
		!reflect.DeepEqual(satisfied.EvidenceRefs, []cognition.EvidenceRef{
			start.Observations[0].EvidenceRef(), transition.Observations[0].EvidenceRef(),
		}) {
		t.Fatalf("satisfied=%+v", satisfied)
	}
}

func catalogCompletionCheck(t *testing.T, investigation Investigation) cognition.CompletionCheckRef {
	t.Helper()
	resolved, err := investigation.Completion().Resolve(investigation.Goal())
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
