package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestSemanticAcceptedFactRequiresActiveOrAncestorScope(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + strings.Repeat("a", 64))
	goal, initial := semanticReplayGraphFixture(t, episode)
	root := initial.Obligations[0]
	a := semanticFactScopeSpec(root, "obligation-sibling-a")
	b := semanticFactScopeSpec(root, "obligation-sibling-b")
	rootSpec := semanticFactScopeSpec(root, root.ID)
	rootSpec.ParentID = ""
	rootSpec.DependsOn = []cognition.ObligationID{a.ID, b.ID}
	graph, err := cognition.NewObligationGraph(1, root.ID, []cognition.ObligationSpec{rootSpec, a, b})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(1); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(b.ID, 1, cognition.ObligationActive); err != nil {
		t.Fatal(err)
	}
	current := cognition.WorldRevision{EpisodeID: episode, Number: 3, SHA256: strings.Repeat("c", 64)}
	for _, scope := range []cognition.ObligationID{root.ID, b.ID} {
		if err := verifySemanticAcceptedFactScope(scope, graph.Snapshot(), current); err != nil {
			t.Fatalf("valid scope %q: %v", scope, err)
		}
	}
	if err := verifySemanticAcceptedFactScope(a.ID, graph.Snapshot(), current); err == nil {
		t.Fatal("selected fact from an inactive sibling scope was accepted")
	}
	_ = goal
}

func TestSemanticAcceptedFactRequiresPriorEvidenceAndExactRenderedAuthority(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + strings.Repeat("b", 64))
	prior := semanticProjectionEvidence(t, episode, 1, "prior", "prior evidence")
	currentEvidence := semanticProjectionEvidence(t, episode, 2, "current", "current evidence")
	current := cognition.WorldRevision{EpisodeID: episode, Number: 2, SHA256: strings.Repeat("f", 64)}
	observations := map[cognition.ObservationID]cognition.EvidenceRef{
		prior.ObservationID: prior, currentEvidence.ObservationID: currentEvidence,
	}
	if err := verifySemanticAcceptedFactSources([]cognition.EvidenceRef{prior}, observations, current); err != nil {
		t.Fatal(err)
	}
	if err := verifySemanticAcceptedFactSources([]cognition.EvidenceRef{currentEvidence}, observations, current); err == nil {
		t.Fatal("accepted fact from the current snapshot revision was accepted")
	}
	content := "code-owned accepted fact"
	digest := digestExactBytes([]byte(content))
	member := queue.CognitionAcceptedFactMaterializationMember{
		Command: taskstate.AddEntryCommand{Content: content},
	}
	selected := contextbuilder.Selection{
		Ref: taskstate.Ref{Hash: digest}, ContentSHA256: digest,
		Authority: taskstate.AuthorityCode,
	}
	if err := verifySemanticAcceptedFactSelection(selected, member); err != nil {
		t.Fatal(err)
	}
	selected.ContentSHA256 = strings.Repeat("d", 64)
	selected.Ref.Hash = selected.ContentSHA256
	if err := verifySemanticAcceptedFactSelection(selected, member); err == nil {
		t.Fatal("coherently changed rendered fact content digest was accepted")
	}
	selected.ContentSHA256, selected.Ref.Hash = digest, digest
	selected.Authority = taskstate.AuthorityModelProposal
	if err := verifySemanticAcceptedFactSelection(selected, member); err == nil {
		t.Fatal("model-authority projected fact was accepted as code-owned")
	}
}

func TestSemanticTerminalActionEventsRequirePostFactPhase55(t *testing.T) {
	for _, status := range []queue.CognitionActionStatus{
		queue.CognitionActionSucceeded, queue.CognitionActionFailed,
	} {
		if !semanticTerminalActionEventTuple(status, 55, 3) {
			t.Fatalf("valid %s terminal action tuple was rejected", status)
		}
		if semanticTerminalActionEventTuple(status, 54, 3) {
			t.Fatalf("legacy pre-fact phase was accepted for %s", status)
		}
	}
}

func semanticFactScopeSpec(
	template cognition.Obligation,
	id cognition.ObligationID,
) cognition.ObligationSpec {
	return cognition.ObligationSpec{
		ID: id, ParentID: template.ID, Desired: template.Desired,
		DependsOn: []cognition.ObligationID{}, SupportingRefs: []cognition.EvidenceRef{},
		CompletionCheck: template.CompletionCheck,
	}
}
