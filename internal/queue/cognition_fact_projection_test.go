package queue

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestProjectedCognitionFactAdmitsOnlyItsExactSelectedSource(t *testing.T) {
	input := cognitionProjectionFitTestInput(t)
	episodeID := input.Episode.EpisodeID
	sourceRevision, err := cognition.NewWorldRevision(episodeID, 1, strings.Repeat("8", 64))
	if err != nil {
		t.Fatal(err)
	}
	currentRevision, err := cognition.NewWorldRevision(episodeID, 2, strings.Repeat("9", 64))
	if err != nil {
		t.Fatal(err)
	}
	source, err := cognition.NewObservation(
		"fact-source", sourceRevision, "record", "Exact source observation.",
	)
	if err != nil {
		t.Fatal(err)
	}
	current, err := cognition.NewObservation(
		"current-state", currentRevision, "state", "Exact current state.",
	)
	if err != nil {
		t.Fatal(err)
	}
	factRef := taskstate.Ref{
		URI: "task:ledger/ledger_fact/entry/fact", Version: "7",
		Hash: strings.Repeat("a", 64), Relation: taskstate.RefSource,
	}
	sourceRef := cognitionEvidenceTaskRefs([]cognition.EvidenceRef{source.EvidenceRef()})[0]
	currentRef := cognitionEvidenceTaskRefs([]cognition.EvidenceRef{current.EvidenceRef()})[0]
	projection := contextbuilder.Projection{Selected: []contextbuilder.Selection{
		{
			ItemID: "accepted-fact", Ref: factRef, SourceRefs: []taskstate.Ref{sourceRef},
			Role: workingset.RoleFact, Authority: taskstate.AuthorityCode,
		},
		{
			ItemID: "current-state", Ref: currentRef, SourceRefs: []taskstate.Ref{},
			Role: workingset.RoleEvidence, Authority: taskstate.AuthorityToolEvidence,
		},
	}}
	completion := []cognition.EvidenceRef{source.EvidenceRef(), current.EvidenceRef()}
	facts := cognitionFactProjectionSources{factRef: {source.EvidenceRef()}}
	model, err := projectedCognitionEvidenceRefs(
		projection, completion, facts, currentRevision, input.Current,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(model) != 2 || model[0] != source.EvidenceRef() || model[1] != current.EvidenceRef() {
		t.Fatalf("projected model evidence=%+v", model)
	}

	changed := projection
	changed.Selected = append([]contextbuilder.Selection{}, projection.Selected...)
	changed.Selected[0].SourceRefs = []taskstate.Ref{sourceRef}
	changed.Selected[0].SourceRefs[0].Hash = strings.Repeat("b", 64)
	if _, err := projectedCognitionEvidenceRefs(
		changed, completion, facts, currentRevision, input.Current,
	); !errors.Is(err, ErrCognitionConflict) {
		t.Fatalf("forged fact source error=%v", err)
	}

	unselected := contextbuilder.Projection{Selected: []contextbuilder.Selection{projection.Selected[1]}}
	model, err = projectedCognitionEvidenceRefs(
		unselected, completion, facts, currentRevision, input.Current,
	)
	if err != nil || len(model) != 1 || model[0] != current.EvidenceRef() {
		t.Fatalf("unselected fact leaked source: model=%+v error=%v", model, err)
	}
}

func TestProjectedCognitionFactRejectsSameRevisionDuplication(t *testing.T) {
	input := cognitionProjectionFitTestInput(t)
	source := input.CompletionEvidence[0]
	factRef := taskstate.Ref{
		URI: "task:ledger/ledger_fact/entry/current-fact", Version: "3",
		Hash: strings.Repeat("d", 64), Relation: taskstate.RefSource,
	}
	projection := contextbuilder.Projection{Selected: []contextbuilder.Selection{{
		ItemID: "current-fact", Ref: factRef,
		SourceRefs: []taskstate.Ref{cognitionEvidenceTaskRefs([]cognition.EvidenceRef{source})[0]},
		Role:       workingset.RoleFact, Authority: taskstate.AuthorityCode,
	}}}
	_, err := projectedCognitionEvidenceRefs(
		projection, input.CompletionEvidence,
		cognitionFactProjectionSources{factRef: {source}}, input.Episode.CurrentRevision, input.Current,
	)
	if !errors.Is(err, ErrCognitionConflict) {
		t.Fatalf("same-revision fact error=%v", err)
	}
}

func TestRetainedRawEvidenceAndLaterFactCoexistOrOverflowLoudly(t *testing.T) {
	input := cognitionProjectionFitTestInput(t)
	source := input.CompletionEvidence[0]
	next, err := cognition.NewWorldRevision(
		input.Episode.EpisodeID, source.Revision.Number+1, strings.Repeat("7", 64),
	)
	if err != nil {
		t.Fatal(err)
	}
	input.Episode.CurrentRevision = next
	factRef := taskstate.Ref{
		URI: "task:ledger/ledger_fact/entry/retained-source-fact", Version: "9",
		Hash: strings.Repeat("6", 64), Relation: taskstate.RefSource,
	}
	factItem := acquireCognitionFitItem(
		t, input.Set, "required-fact", workingset.RoleFact, 95, factRef, 256,
	)
	input.Materials = append(input.Materials, contextbuilder.Material{
		ItemID: factItem.Item.ID, CurrentRef: factItem.Item.Ref,
		SourceRefs: []taskstate.Ref{cognitionEvidenceTaskRefs([]cognition.EvidenceRef{source})[0]},
		Authority:  taskstate.AuthorityCode, Content: strings.Repeat("f", 256), ByteCost: 256,
	})
	input.Spec.Required = append(input.Spec.Required, contextbuilder.Selector{
		ID: "fact", Role: workingset.RoleFact, MinItems: 1, MaxItems: 1,
	})
	input.Spec.Optional = []contextbuilder.Selector{}
	input.Spec.Required = append(input.Spec.Required, contextbuilder.Selector{
		ID: "retained-raw", Role: workingset.RoleEvidence, MinItems: 1, MaxItems: 1,
	})
	input.Spec.MaxItems = 3
	input.FactSources = cognitionFactProjectionSources{factRef: {source}}
	input.Initial, err = contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: input.WorkID, Spec: input.Spec, WorkingSet: input.Set, Materials: input.Materials,
	})
	if err != nil {
		t.Fatal(err)
	}
	fit, err := fitCognitionPolicyProjection(input)
	if err != nil {
		t.Fatal(err)
	}
	roles := make(map[workingset.Role]int)
	for _, selected := range fit.Projection.Selected {
		roles[selected.Role]++
	}
	if roles[workingset.RoleFact] != 1 || roles[workingset.RoleEvidence] != 1 ||
		len(fit.Snapshot.EvidenceRefs()) != 1 || fit.Snapshot.EvidenceRefs()[0] != source {
		t.Fatalf("retained raw/fact projection roles=%v evidence=%+v", roles, fit.Snapshot.EvidenceRefs())
	}
	input.Budget.MaxInputBytes = fit.Envelope.Bytes - 1
	if _, err := fitCognitionPolicyProjection(input); !errors.Is(err, ErrCognitionEnvelopeBudget) {
		t.Fatalf("required retained raw/fact overflow error=%v", err)
	}
}
