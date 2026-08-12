package cognitiongauntlet

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestSemanticSnapshotEvidenceIsExactSelectedProjectionLineage(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + strings.Repeat("a", 64))
	first := semanticProjectionEvidence(t, episode, 1, "first", "first clue")
	second := semanticProjectionEvidence(t, episode, 2, "second", "second clue")
	projection := contextbuilder.Projection{Selected: []contextbuilder.Selection{
		semanticProjectionEvidenceSelection(second),
		semanticProjectionEvidenceSelection(first),
	}}
	observations := map[cognition.ObservationID]cognition.EvidenceRef{
		first.ObservationID: first, second.ObservationID: second,
	}
	want := []cognition.EvidenceRef{second, first}
	if err := verifySemanticSnapshotEvidenceRefs(
		projection, want, observations, nil, cognition.WorldRevision{EpisodeID: episode, Number: 3, SHA256: strings.Repeat("e", 64)}, cognition.ObligationGraphSnapshot{},
	); err != nil {
		t.Fatal(err)
	}
	for name, changed := range map[string][]cognition.EvidenceRef{
		"reordered": {first, second},
		"missing":   {second},
		"extra":     {second, first, second},
	} {
		t.Run(name, func(t *testing.T) {
			if err := verifySemanticSnapshotEvidenceRefs(
				projection, changed, observations, nil, cognition.WorldRevision{EpisodeID: episode, Number: 3, SHA256: strings.Repeat("e", 64)}, cognition.ObligationGraphSnapshot{},
			); err == nil {
				t.Fatal("snapshot evidence outside exact selected order was accepted")
			}
		})
	}
	projection.Selected[0].Role = workingset.RoleFact
	projection.Selected[0].SourceRefs = []taskstate.Ref{
		semanticProjectionEvidenceSelection(first).Ref,
	}
	err := verifySemanticSnapshotEvidenceRefs(
		projection, want, observations, nil, cognition.WorldRevision{EpisodeID: episode, Number: 3, SHA256: strings.Repeat("e", 64)}, cognition.ObligationGraphSnapshot{},
	)
	if err == nil || !strings.Contains(err.Error(), "materialization") {
		t.Fatal("selected fact without frozen materialization evidence was accepted")
	}
}

func semanticProjectionEvidence(
	t *testing.T,
	episode cognition.EpisodeID,
	revision uint64,
	id cognition.ObservationID,
	content string,
) cognition.EvidenceRef {
	t.Helper()
	observation, err := cognition.NewObservation(id, cognition.WorldRevision{
		EpisodeID: episode, Number: revision,
		SHA256: strings.Repeat(string(rune('b')+rune(revision)), 64),
	}, "clue", content)
	if err != nil {
		t.Fatal(err)
	}
	return observation.EvidenceRef()
}

func semanticProjectionEvidenceSelection(
	ref cognition.EvidenceRef,
) contextbuilder.Selection {
	return contextbuilder.Selection{
		ItemID: workingset.ItemID("item-" + string(ref.ObservationID)),
		Ref: taskstate.Ref{
			URI: "cognition:episode/" + string(ref.Revision.EpisodeID) +
				"/observation/" + string(ref.ObservationID),
			Version: strconv.FormatUint(ref.Revision.Number, 10), Hash: ref.SHA256,
			Relation: taskstate.RefEvidence,
		},
		Role: workingset.RoleEvidence,
	}
}
