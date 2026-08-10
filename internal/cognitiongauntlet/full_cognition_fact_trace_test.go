package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

type factRuntimeSnapshot struct {
	Goal              cognition.GoalExpression       `json:"goal"`
	CurrentRevision   cognition.WorldRevision        `json:"current_revision"`
	CurrentObligation cognition.Obligation           `json:"current_obligation"`
	ActionCatalog     cognition.ActionCatalog        `json:"action_catalog"`
	Attempt           cognition.AttemptRef           `json:"attempt"`
	ContextProjection cognition.ContextProjectionRef `json:"context_projection"`
	Budget            cognition.RuntimeBudget        `json:"budget"`
	EvidenceRefs      []cognition.EvidenceRef        `json:"evidence_refs"`
}

func traceSnapshotByCall(t *testing.T, trace productionTrace) map[int64]factRuntimeSnapshot {
	t.Helper()
	result := make(map[int64]factRuntimeSnapshot)
	for _, record := range trace.Records {
		if record.Kind != "runtime_snapshot" {
			continue
		}
		snapshot := factRuntimeSnapshot{}
		if err := decodeProductionPayload(record.Payload, &snapshot, "fact runtime snapshot"); err != nil {
			t.Fatal(err)
		}
		if snapshot.CurrentRevision.Number == 0 || snapshot.CurrentRevision.EpisodeID != trace.Header.EpisodeID {
			t.Fatal("fact runtime snapshot revision authority is invalid")
		}
		result[record.CallOrdinal] = snapshot
	}
	return result
}

func assertNoPrivateModelVocabulary(t *testing.T, content string) {
	t.Helper()
	lower := strings.ToLower(content)
	for _, forbidden := range []string{"gauntlet", "oracle", "benchmark", "witness", "hidden"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("model-visible content contains private vocabulary %q", forbidden)
		}
	}
}
