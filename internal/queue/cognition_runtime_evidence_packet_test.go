package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestActiveEvidenceSelectionExcludesReadySiblingEvidence(t *testing.T) {
	active := cognition.EvidenceRef{ObservationID: "active-evidence"}
	readyLeft := cognition.EvidenceRef{ObservationID: "ready-left-evidence"}
	readyRight := cognition.EvidenceRef{ObservationID: "ready-right-evidence"}
	selected, err := selectActiveCognitionEvidence(cognition.ObligationGraphSnapshot{Obligations: []cognition.Obligation{
		{ID: "active", Status: cognition.ObligationActive, SupportingRefs: []cognition.EvidenceRef{active}},
		{ID: "ready-left", Status: cognition.ObligationReady, SupportingRefs: []cognition.EvidenceRef{readyLeft}},
		{ID: "ready-right", Status: cognition.ObligationReady, SupportingRefs: []cognition.EvidenceRef{readyRight}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := selected[active]; !exists || len(selected) != 1 {
		t.Fatalf("selected evidence=%v, want only active obligation evidence", selected)
	}
	if _, exists := selected[readyLeft]; exists {
		t.Fatal("left ready sibling evidence entered the active call")
	}
	if _, exists := selected[readyRight]; exists {
		t.Fatal("right ready sibling evidence entered the active call")
	}
}
