package worker

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptGuidedRepairRecordsZeroDeltaWithoutAnotherSemanticCall(t *testing.T) {
	t.Parallel()
	const current = "function Normalize(value: number): number { return value; }"
	block := assemblyline.SourceBlock{
		ID:        "feature.normalize",
		Signature: "function Normalize(value: number): number",
		Contract:  "Return the normalized value.",
		API:       "function Normalize(value: number): number",
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: t.Context(), MaxAttempts: exactSemanticLeafCalls,
		Execute: testPortableExecutor(func(scope string, _ string, _ string) (string, error) {
			calls++
			switch scope {
			case "portable_semantic_worker":
				return "Return the local numeric parameter directly.", nil
			case "portable_fragment_worker":
				return current, nil
			default:
				return "", fmt.Errorf("unexpected scope %q", scope)
			}
		}),
	}
	_, err := convergeDirectCodingTypeScriptGuidedRepairWithRuntime(
		runtime, "guidance", "executor", directCodingTypeScriptRepairEvents{},
		block, false, "TypeScript function syntax", "", current, nil,
		"TYPESCRIPT_DIAGNOSTIC: exact compiler failure", nil,
	)
	if !errors.Is(err, errDirectCodingTypeScriptUnchangedCorrection) || calls != 2 {
		t.Fatalf("zero delta error=%v calls=%d", err, calls)
	}
}
