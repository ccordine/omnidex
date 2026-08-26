package assemblyline

import (
	"strings"
	"testing"
)

func TestTypeScriptCorrectionAdmitsLegitimateDeclarationBeyondLegacyByteCeilings(t *testing.T) {
	t.Parallel()

	contract := TypeScriptFunctionContract{Signature: "function measurePayload(): number"}
	current := largeTypeScriptFunction(40*1024, "return payload.length;")
	if len(current) <= 32*1024 {
		t.Fatalf("fixture=%d bytes; expected it to exceed every retired microscopic ceiling", len(current))
	}
	if _, err := ParseTypeScriptFunction(contract, current); err != nil {
		t.Fatalf("parse legitimate large declaration: %v", err)
	}

	job, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		Language:           "typescript",
		Signature:          contract.Signature,
		CurrentDeclaration: current,
		RepairGuidance:     "Subtract one from the returned payload length.",
	})
	if err != nil {
		t.Fatalf("create correction for legitimate large declaration: %v", err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatalf("render correction for legitimate large declaration: %v", err)
	}
	if schema != nil {
		t.Fatal("whole-declaration correction unexpectedly requested structured output")
	}
	if !strings.Contains(prompt, "EXACT_MUTABLE_SOURCE_JSON:") ||
		!strings.Contains(prompt, "return payload.length;") {
		t.Fatal("correction prompt omitted retained declaration authority")
	}

	corrected := largeTypeScriptFunction(40*1024, "return payload.length - 1;")
	result := PortableResult{JobID: job.ID, Candidate: corrected}
	if err := result.ValidateFor(job); err != nil {
		t.Fatalf("validate legitimate large correction result: %v", err)
	}
	projection, err := ProjectTypeScriptFunctionModelResponse(contract, result.Candidate)
	if err != nil {
		t.Fatalf("decode legitimate large correction result: %v", err)
	}
	if projection.Source != corrected {
		t.Fatal("decoded correction changed accepted source")
	}
}

func TestTypeScriptLengthIsNotSourceInvalidityAndPortableTransportKeepsRunawayCeiling(t *testing.T) {
	t.Parallel()

	contract := TypeScriptFunctionContract{Signature: "function measurePayload(): number"}
	runaway := largeTypeScriptFunction(maxPortableResourceBytes, "return payload.length;")
	if len(runaway) <= maxPortableResourceBytes {
		t.Fatalf("runaway fixture=%d bytes; expected >%d", len(runaway), maxPortableResourceBytes)
	}
	if _, err := ParseTypeScriptFunction(contract, runaway); err != nil {
		t.Fatalf("source length became a TypeScript validity rule: %v", err)
	}

	_, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		Language:           "typescript",
		Signature:          contract.Signature,
		CurrentDeclaration: runaway,
		RepairGuidance:     "Return the payload length.",
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("runaway correction job error=%v", err)
	}
}

func largeTypeScriptFunction(payloadBytes int, result string) string {
	return "function measurePayload(): number {\n" +
		"  const payload = \"" + strings.Repeat("x", payloadBytes) + "\";\n" +
		"  " + result + "\n" +
		"}"
}
