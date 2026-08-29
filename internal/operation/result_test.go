package operation

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
)

func TestRejectedOperationErrorPreservesCauseAndIdentity(t *testing.T) {
	t.Parallel()
	cause := errors.New("invalid deterministic operation")
	rejected := Reject(cause)
	if rejected == nil || !IsRejected(rejected) || !errors.Is(rejected, cause) {
		t.Fatalf("rejected=%v", rejected)
	}
	if Reject(rejected) != rejected {
		t.Fatal("rejection wrapping is not idempotent")
	}
	if Reject(nil) != nil || IsRejected(cause) {
		t.Fatal("ordinary errors were reclassified as rejected operations")
	}
}

func TestResultCarriesOnlyDeterministicOperationEvidence(t *testing.T) {
	t.Parallel()
	result := Result{
		Summary: "verified",
		Output:  map[string]any{"succeeded": true},
		Evidence: []evidence.Record{{
			Kind: evidence.KindTestResult,
		}},
	}
	if result.Summary != "verified" || result.Output["succeeded"] != true ||
		len(result.Evidence) != 1 {
		t.Fatalf("result=%#v", result)
	}
}
