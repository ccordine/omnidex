package queue

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestResolveCognitionProjectionEvidenceRefUsesExactProductionTaskReference(t *testing.T) {
	revision := cognition.WorldRevision{
		EpisodeID: "episode-" + cognition.EpisodeID(strings.Repeat("a", 64)),
		Number:    3, SHA256: strings.Repeat("b", 64),
	}
	observation, err := cognition.NewObservation(
		"observation-projected", revision, "clue", "exact clue",
	)
	if err != nil {
		t.Fatal(err)
	}
	evidence := observation.EvidenceRef()
	selected := taskstate.Ref{
		URI: "cognition:episode/" + string(revision.EpisodeID) +
			"/observation/" + string(evidence.ObservationID),
		Version: strconv.FormatUint(revision.Number, 10), Hash: evidence.SHA256,
		Relation: taskstate.RefEvidence,
	}
	got, err := ResolveCognitionProjectionEvidenceRef(selected, []cognition.EvidenceRef{evidence})
	if err != nil || got != evidence {
		t.Fatalf("resolved=%+v err=%v", got, err)
	}
	for name, mutate := range map[string]func(*taskstate.Ref){
		"URI":      func(value *taskstate.Ref) { value.URI += "-changed" },
		"version":  func(value *taskstate.Ref) { value.Version = "4" },
		"hash":     func(value *taskstate.Ref) { value.Hash = strings.Repeat("f", 64) },
		"relation": func(value *taskstate.Ref) { value.Relation = taskstate.RefSource },
	} {
		t.Run(name, func(t *testing.T) {
			changed := selected
			mutate(&changed)
			if _, err := ResolveCognitionProjectionEvidenceRef(
				changed, []cognition.EvidenceRef{evidence},
			); err == nil {
				t.Fatal("changed projection evidence reference was accepted")
			}
		})
	}
	ambiguous := evidence
	ambiguous.Revision.SHA256 = strings.Repeat("c", 64)
	if _, err := ResolveCognitionProjectionEvidenceRef(
		selected, []cognition.EvidenceRef{evidence, ambiguous},
	); err == nil {
		t.Fatal("ambiguous projection evidence task reference was accepted")
	}
}
