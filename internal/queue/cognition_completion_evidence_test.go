package queue

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestMissingCognitionCompletionEvidencePreservesExactNewOrder(t *testing.T) {
	first := cognition.EvidenceRef{
		ObservationID: "one", Revision: cognition.WorldRevision{
			EpisodeID: "episode", Number: 1, SHA256: cognitionTestDigest("1"),
		}, SHA256: cognitionTestDigest("a"),
	}
	second := cognition.EvidenceRef{
		ObservationID: "two", Revision: cognition.WorldRevision{
			EpisodeID: "episode", Number: 2, SHA256: cognitionTestDigest("2"),
		}, SHA256: cognitionTestDigest("b"),
	}
	third := cognition.EvidenceRef{
		ObservationID: "three", Revision: cognition.WorldRevision{
			EpisodeID: "episode", Number: 3, SHA256: cognitionTestDigest("3"),
		}, SHA256: cognitionTestDigest("c"),
	}
	got := missingCognitionCompletionEvidence(
		[]cognition.EvidenceRef{first}, []cognition.EvidenceRef{first, second, second, third},
	)
	want := []cognition.EvidenceRef{second, third}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("missing=%+v want=%+v", got, want)
	}
}
