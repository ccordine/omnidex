package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func TestOpaqueSourceCorrectionUsesLineFramingAndExplicitBudget(t *testing.T) {
	t.Parallel()
	call := exactStationCall{
		WorkID: "work.opaque-correction", WorkKind: assemblyline.WorkFragmentGeneration,
		Prompt: "choose", ContextTokens: 8192, MaxOutputTokens: 8,
		SingleLine: true,
	}
	prepared, err := prepareExactStationCall(call, "fixture-model", nil)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.RawTextStopSequence != llm.ExactPreparedLineStopV1 {
		t.Fatalf("opaque correction stop=%q", prepared.RawTextStopSequence)
	}
	if prepared.OutputLimitMode != llm.ExactPreparedOutputLimitExplicit ||
		prepared.MaxOutputTokens != call.MaxOutputTokens {
		t.Fatalf(
			"opaque correction limit=(%q,%d), want explicit budget %d",
			prepared.OutputLimitMode, prepared.MaxOutputTokens, call.MaxOutputTokens,
		)
	}
}
