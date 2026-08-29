package worker

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRepresentedPersistedSimulationEventsRemovesOnlyExactPendingSuffix(t *testing.T) {
	const (
		persistedTransitionID = "11111111111111111111111111111111"
		pendingTransitionID   = "22222222222222222222222222222222"
		duplicateEvent        = "The same bell sounds twice."
	)
	preparation := roleplay.SimulationTurnAuthority{PendingTransition: &roleplay.SimulationTransitionResult{
		OperationID: pendingTransitionID, NarrativeEvents: []string{duplicateEvent},
	}}
	responder := roleplay.SimulationResponderAuthority{
		NarrativeProjection: roleplay.NarrativeSimulationProjection{
			RecentEvents: []string{duplicateEvent, duplicateEvent},
		},
		NarrativeAuthority: roleplay.SimulationNarrativeAuthority{
			TransitionIDs: []string{persistedTransitionID, pendingTransitionID},
		},
	}

	persisted, err := representedPersistedSimulationEvents(preparation, responder)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted, []string{duplicateEvent}) {
		t.Fatalf("persisted events=%#v", persisted)
	}

	responder.NarrativeAuthority.TransitionIDs = []string{
		pendingTransitionID, persistedTransitionID,
	}
	_, err = representedPersistedSimulationEvents(preparation, responder)
	if err == nil || !strings.Contains(err.Error(), "pending transition suffix") {
		t.Fatalf("mismatched pending transition error=%v", err)
	}
}
