package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func liveRawFragmentCallError(calls []liveRawFragmentProviderCall, index int) error {
	if index < 0 || index >= len(calls) {
		return nil
	}
	return calls[index].Err
}

func liveRawFragmentCallContent(calls []liveRawFragmentProviderCall, index int) string {
	if index < 0 || index >= len(calls) {
		return ""
	}
	return calls[index].Generation.Content
}

func assertLiveGuidedTSXRepairQualification(
	t *testing.T,
	fixtureName string,
	functionName string,
	target assemblyline.SourceBlock,
	available string,
	current string,
	verifierSource string,
	behaviorTest string,
	matcher string,
	supplementSentinel string,
	dialect string,
	feedback string,
	guidance string,
	source string,
	calls []liveRawFragmentProviderCall,
) {
	t.Helper()
	if len(calls) != 2 {
		t.Fatalf("guided TSX qualification received %d provider calls", len(calls))
	}
	if source == current || source == "" || source != strings.TrimSpace(source) {
		t.Fatalf("guided TSX repair did not return one changed exact source declaration: %q", source)
	}
	if !strings.Contains(source, "\n") {
		t.Fatalf("guided TSX repair did not preserve physical source line boundaries: %q", source)
	}
	fragment, err := assemblyline.ParseTypeScriptFunction(
		assemblyline.TypeScriptFunctionContract{
			Signature: target.Signature, TSX: true, Policy: target.Policy,
		},
		source,
	)
	if err != nil {
		t.Fatalf("parse guided TSX repair: %v", err)
	}
	if fragment.Name != functionName {
		t.Fatalf("guided TSX repair function=%q want=%q", fragment.Name, functionName)
	}
	lowerSource := strings.ToLower(fragment.Source)
	for _, forbiddenProjectionLabel := range []string{
		"regular expression pattern", "plain text", "source text", "source flag", "u+0020", "hexadecimal",
	} {
		if strings.Contains(lowerSource, forbiddenProjectionLabel) {
			t.Fatalf("guided TSX repair copied projection label %q into source: %s", forbiddenProjectionLabel, source)
		}
	}

	guidancePrompt := calls[0].Prepared.Prompt
	correctionPrompt := calls[1].Prepared.Prompt
	if !strings.Contains(guidancePrompt, "EXACT_VALIDATION_FAILURE:\n"+feedback) ||
		!strings.Contains(guidancePrompt, available) ||
		strings.Contains(guidancePrompt, verifierSource) ||
		strings.Contains(guidancePrompt, supplementSentinel) ||
		strings.Contains(guidancePrompt, target.Contract) {
		t.Fatalf("repair-guidance prompt crossed its exact authority boundary:\n%s", guidancePrompt)
	}
	if !strings.Contains(correctionPrompt, "EXACT_MUTABLE_SOURCE_JSON:") ||
		!strings.Contains(correctionPrompt, "REQUIRED_SOURCE_TRANSFORMATION:\n"+guidance) ||
		strings.Contains(correctionPrompt, "EXACT_VALIDATION_FAILURE:") ||
		strings.Contains(correctionPrompt, verifierSource) ||
		strings.Contains(correctionPrompt, supplementSentinel) ||
		strings.Contains(correctionPrompt, target.Contract) ||
		strings.Contains(correctionPrompt, dialect) ||
		strings.Contains(correctionPrompt, available) {
		t.Fatalf("repair-executor prompt crossed its exact authority boundary:\n%s", correctionPrompt)
	}
	for index, call := range calls {
		if strings.Contains(call.Prepared.Prompt, matcher) ||
			strings.Contains(call.Generation.Content, matcher) ||
			strings.Contains(call.Generation.Content, supplementSentinel) {
			t.Fatalf("guided repair call %d retained raw matcher %q", index+1, matcher)
		}
	}
	requireLiveGuidedTSXBehavior(
		t, fixtureName, available, functionName, current, source, behaviorTest,
	)
}
