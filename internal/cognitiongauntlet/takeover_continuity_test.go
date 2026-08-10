package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestTakeoverContinuityRequiresNewAuthorityAndIdenticalSemanticState(t *testing.T) {
	t.Parallel()
	before := testSemanticPreCallCheckpoint(1, "worker-before", "projection-before", "snapshot-before")
	after := testSemanticPreCallCheckpoint(2, "worker-after", "projection-after", "snapshot-after")

	proof, err := NewTakeoverContinuityProof(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if proof.Before.Bound.Projection == proof.After.Bound.Projection ||
		proof.Before.Bound.SnapshotSHA256 == proof.After.Bound.SnapshotSHA256 {
		t.Fatal("valid takeover proof reused attempt-bound authority")
	}
	if proof.Before.SemanticSHA256 != proof.After.SemanticSHA256 ||
		proof.Before.ProjectionRenderedSHA256 != proof.After.ProjectionRenderedSHA256 {
		t.Fatal("valid takeover proof changed attempt-normalized state")
	}
}

func TestTakeoverContinuityRejectsAuthorityOrSemanticSubstitution(t *testing.T) {
	t.Parallel()
	before := testSemanticPreCallCheckpoint(1, "worker-before", "projection-before", "snapshot-before")
	validAfter := testSemanticPreCallCheckpoint(2, "worker-after", "projection-after", "snapshot-after")

	for name, mutate := range map[string]func(*SemanticPreCallCheckpoint){
		"attempt gap": func(value *SemanticPreCallCheckpoint) {
			value.Bound.Attempt.Attempt = 3
		},
		"same worker": func(value *SemanticPreCallCheckpoint) {
			value.Bound.Attempt.WorkerID = before.Bound.Attempt.WorkerID
		},
		"another step": func(value *SemanticPreCallCheckpoint) {
			value.Bound.Attempt.StepID++
		},
		"reused projection": func(value *SemanticPreCallCheckpoint) {
			value.Bound.Projection = before.Bound.Projection
			value.ProjectionRenderedSHA256 = before.ProjectionRenderedSHA256
		},
		"reused snapshot": func(value *SemanticPreCallCheckpoint) {
			value.Bound.SnapshotSHA256 = before.Bound.SnapshotSHA256
		},
		"changed rendered context": func(value *SemanticPreCallCheckpoint) {
			value.ProjectionRenderedSHA256 = strings.Repeat("d", 64)
			value.Bound.Projection.SHA256 = value.ProjectionRenderedSHA256
		},
		"changed semantic state": func(value *SemanticPreCallCheckpoint) {
			value.SemanticSHA256 = strings.Repeat("e", 64)
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			after := validAfter
			mutate(&after)
			if _, err := NewTakeoverContinuityProof(before, after); err == nil {
				t.Fatal("takeover continuity accepted substituted state")
			}
		})
	}
}

func testSemanticPreCallCheckpoint(
	attempt uint64,
	worker string,
	projectionID cognition.ContextProjectionID,
	snapshotIdentity string,
) SemanticPreCallCheckpoint {
	renderedSHA := strings.Repeat("a", 64)
	return SemanticPreCallCheckpoint{
		Schema: SemanticPreCallCheckpointSchemaV1,
		Bound: PreCallBoundAuthority{
			Attempt: cognition.AttemptRef{
				JobID: 71, Generation: 4, StepID: 9, Attempt: attempt, WorkerID: worker,
			},
			Projection: cognition.ContextProjectionRef{
				ID: projectionID, SHA256: renderedSHA,
				WorkingSetID: "working-set-takeover", WorkingSetVersion: 8,
				RendererVersion: "omnidex.context-material-json.v1",
			},
			SnapshotSHA256: digestForTest(snapshotIdentity),
		},
		SemanticSHA256: digestForTest("semantic-state"), ProjectionRenderedSHA256: renderedSHA,
	}
}

func digestForTest(value string) string {
	if value == "snapshot-before" {
		return strings.Repeat("b", 64)
	}
	if value == "snapshot-after" {
		return strings.Repeat("c", 64)
	}
	return strings.Repeat("f", 64)
}
