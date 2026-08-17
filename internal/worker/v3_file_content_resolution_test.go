package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTargetTreeBindingDerivesForcedSinglePairWithoutModelCall(t *testing.T) {
	specification := assemblyline.ApplicationSpecification{Requirements: []assemblyline.Requirement{
		{ID: "requirement_001", SourceQuote: "display a count"},
		{ID: "requirement_002", SourceQuote: "increment the count"},
	}}
	tree, err := deriveDirectCodingTargetTreeBindings(
		specification,
		assemblyline.TargetTree{
			StackID: genericTypeScriptBrowserAdapter,
			Paths:   []string{"src/Counter.tsx", "tests/Counter.test.tsx"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Bindings) != 2 {
		t.Fatalf("bindings=%+v", tree.Bindings)
	}
	for _, requirement := range specification.Requirements {
		files, err := tree.RequirementFiles(requirement.ID)
		if err != nil {
			t.Fatal(err)
		}
		if files.ImplementationPath != "src/Counter.tsx" || files.VerificationPath != "tests/Counter.test.tsx" {
			t.Fatalf("requirement %q files=%+v", requirement.ID, files)
		}
	}
}

func TestTargetTreeBindingRefusesNonForcedTopologyInsteadOfCallingAModel(t *testing.T) {
	specification := assemblyline.ApplicationSpecification{Requirements: []assemblyline.Requirement{
		{ID: "requirement_001", SourceQuote: "one behavior"},
	}}
	_, err := deriveDirectCodingTargetTreeBindings(specification, assemblyline.TargetTree{
		StackID: genericTypeScriptBrowserAdapter,
		Paths:   []string{"src/App.tsx", "src/Feature.tsx", "tests/Feature.test.tsx"},
	})
	if err == nil || !strings.Contains(err.Error(), "explicit artifact coordination is required") {
		t.Fatalf("error=%v", err)
	}
}
