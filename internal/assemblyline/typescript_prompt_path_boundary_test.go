package assemblyline

import (
	"strings"
	"testing"
)

func TestTypeScriptFragmentPromptsStateTheEnforcedPathBlindSourceGrammar(t *testing.T) {
	initial, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Dialect:   "TypeScript",
		Signature: "function FormatCalendarDate(month: number, day: number): string",
		Contract:  "Return the supplied month and day with inert solidus punctuation between them.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(initial, typeScriptPathBlindSourceRule) != 1 {
		t.Fatalf("initial prompt does not carry one exact path-blind source rule: %q", initial)
	}
	if err := ValidatePathFreeModelContext("TypeScript initial prompt", initial); err != nil {
		t.Fatalf("generic source rule leaked a filesystem identity: %v", err)
	}

	correctionJob, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		Language:           "typescript",
		Signature:          "function PrefixCommand(name: string): string",
		CurrentDeclaration: "function PrefixCommand(name: string): string { return name; }",
		RepairGuidance:     "Prefix the supplied name with inert solidus punctuation.",
	})
	if err != nil {
		t.Fatal(err)
	}
	correction, err := RenderPortableJob(correctionJob)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(correction, typeScriptPathBlindSourceRule) != 1 {
		t.Fatalf("correction prompt does not carry one exact path-blind source rule: %q", correction)
	}
}

func TestTypeScriptSourcePathBoundaryAcceptsRuntimePunctuationWithoutAcceptingPaths(t *testing.T) {
	accepted := []string{
		`function FormatCalendarDate(month: number, day: number): string {
  return String(month) + String.fromCharCode(47) + String(day);
}`,
		`function PrefixCommand(name: string): string {
  return String.fromCharCode(47) + name;
}`,
	}
	for index, source := range accepted {
		if err := ValidatePathFreeSourceModelContext("unrelated source fixture", source); err != nil {
			t.Fatalf("accepted fixture %d failed: %v", index, err)
		}
	}

	rejected := []string{
		`function Root(): string { return "/"; }`,
		`function Config(): string { return "/etc/service"; }`,
		`function Module(): string { return "./widget"; }`,
		`function Pair(): string { return "left/right"; }`,
		"function Note(): void { // inspect /var/data\n}",
	}
	for index, source := range rejected {
		if err := ValidatePathFreeSourceModelContext("forbidden source fixture", source); err == nil {
			t.Fatalf("rejected fixture %d unexpectedly crossed the path boundary: %q", index, source)
		}
	}
}
