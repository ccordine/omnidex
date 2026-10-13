package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestConstructionContextPreservesOnlyLiteralSourceConstraints(t *testing.T) {
	input := ConstructionContextInput{UserRequest: "Build a dependency-free Python module temperature.py that converts temperatures. Include automated tests and a short README showing usage."}
	result, err := DecodeConstructionContext(input, "language\tPython\noutput_file\ttemperature.py\npackaging\tdependency-free\nverification\tautomated tests\ndocumentation\ta short README showing usage")
	if err != nil || len(result.Facts) != 5 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	job, err := NewConstructionContextJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil || !strings.Contains(prompt, input.UserRequest) {
		t.Fatalf("prompt=%q error=%v", prompt, err)
	}
	if _, err := SemanticUncertaintyContractForWorkKind(job.Kind); err != nil {
		t.Fatal(err)
	}
}

func TestConstructionContextRejectsInventedWholeGoalAndDuplicateQuotes(t *testing.T) {
	input := ConstructionContextInput{UserRequest: "Build a Python module with tests."}
	for _, output := range []string{
		"language\tRust", "planning\tPython", "language\t" + input.UserRequest,
		"language\tPython\npackaging\tPython", "output_file\t../secret", "language Python",
	} {
		if _, err := DecodeConstructionContext(input, output); err == nil {
			t.Fatalf("invalid context accepted: %q", output)
		}
	}
	result, err := DecodeConstructionContext(input, NoConstructionConstraints)
	if err != nil || len(result.Facts) != 0 {
		t.Fatalf("absence=%+v err=%v", result, err)
	}
}

func TestConstructionContextRestoresOnlyKnownArtifactProvenance(t *testing.T) {
	input := ConstructionContextInput{UserRequest: "Build Python module ARTIFACT_1 with tests."}
	result, err := DecodeConstructionContext(input, "output_file\tARTIFACT_1")
	if err != nil {
		t.Fatal(err)
	}
	quote, err := RestoreArtifactIdentities(result.Facts[0].Quote, []ArtifactIdentity{{Token: "ARTIFACT_1", Value: "temperature.py"}})
	if err != nil || quote != "temperature.py" {
		t.Fatalf("quote=%q error=%v", quote, err)
	}
	if _, err := RestoreArtifactIdentities(result.Facts[0].Quote, nil); err == nil {
		t.Fatal("unbound artifact gained authority")
	}
}

func TestConstructionContextPreservesMultilineLiteralSourceConstraints(t *testing.T) {
	for _, fixture := range []struct {
		name  string
		input string
		raw   string
		facts []ConstructionFact
	}{
		{
			name:  "observed verification continuation",
			input: "Build a client. Verify the\n completed flows: actual ARTIFACT_3 requests, phone notifications and approval, cancellation, denial, and preservation of local authentication. Deliver the phone APK.",
			raw:   "verification\tVerify the\n completed flows: actual ARTIFACT_3 requests, phone notifications and approval, cancellation, denial, and preservation of local authentication.\noutput_file\tphone APK",
			facts: []ConstructionFact{
				{Kind: "verification", Quote: "Verify the\n completed flows: actual ARTIFACT_3 requests, phone notifications and approval, cancellation, denial, and preservation of local authentication."},
				{Kind: "output_file", Quote: "phone APK"},
			},
		},
		{
			name:  "unrelated documentation continuation",
			input: "Build a Go library. Include a README\n\n  showing serialization examples\n  and error handling.",
			raw:   "language\tGo\ndocumentation\tInclude a README\n\n  showing serialization examples\n  and error handling",
			facts: []ConstructionFact{
				{Kind: "language", Quote: "Go"},
				{Kind: "documentation", Quote: "Include a README\n\n  showing serialization examples\n  and error handling"},
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			result, err := DecodeConstructionContext(ConstructionContextInput{UserRequest: fixture.input}, fixture.raw)
			if err != nil || !reflect.DeepEqual(result.Facts, fixture.facts) {
				t.Fatalf("result=%+v error=%v, want facts=%+v", result, err, fixture.facts)
			}
		})
	}
}

func TestConstructionContextRejectsInvalidMultilineSourceConstraints(t *testing.T) {
	input := ConstructionContextInput{UserRequest: "Build a Python module. Include tests\n  covering invalid inputs and a README."}
	for _, raw := range []string{
		"Include tests\n  covering invalid inputs",
		"verification\tInclude tests\n covering invalid inputs",
		"verification\tInclude tests\n  covering invented inputs",
		"verification\tInclude tests\n  covering invalid inputs\ndocumentation\tInclude tests\n  covering invalid inputs",
		"verification\tInclude tests\n  covering invalid inputs\nplanning\tPython",
		"verification\t" + input.UserRequest,
		"language\tPython\n",
	} {
		if _, err := DecodeConstructionContext(input, raw); err == nil {
			t.Fatalf("invalid multiline context accepted: %q", raw)
		}
	}
}
