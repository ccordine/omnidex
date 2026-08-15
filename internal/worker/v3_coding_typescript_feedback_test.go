package worker

import (
	"strings"
	"testing"
)

func TestTypeScriptModelFailureRejectsGenericPathAndFileIdentities(t *testing.T) {
	t.Parallel()
	for _, identity := range []string{
		"/tmp/config.yaml",
		`C:\temp\config.yaml`,
		"config/settings.yaml",
		"config.yaml",
		"go.mod",
		".env",
		"runner.js",
		"CONFIG.YAML",
		"First.GO",
		"mixed.JsOn",
		".ENV",
	} {
		raw := "AssertionError: expected " + identity + " to be valid"
		feedback := directCodingTypeScriptTestModelFailure(raw)
		if strings.Contains(feedback, identity) {
			t.Fatalf("feedback leaked path or file identity %q: %q", identity, feedback)
		}
		if feedback != "Validation failed without a concise function-owned diagnostic." {
			t.Fatalf("path-bearing diagnostic %q produced feedback %q", raw, feedback)
		}
	}
}

func TestTypeScriptModelFailurePreservesLegitimatePathFreeDiagnostics(t *testing.T) {
	t.Parallel()
	for _, diagnostic := range []string{
		"AssertionError: expected version 1.2.3 to equal 1.2.4",
		"TypeError: expected configuration value to be defined",
		"Error TS2322: Type string is not assignable to type number",
	} {
		feedback := directCodingTypeScriptTestModelFailure(diagnostic)
		if feedback != diagnostic {
			t.Fatalf("path-free diagnostic %q became %q", diagnostic, feedback)
		}
	}
}
