package assemblyline

import (
	"errors"
	"testing"
)

func TestPortableResponseMaximumRegistryIsExhaustive(t *testing.T) {
	seen := make(map[WorkKind]struct{}, len(AllWorkKinds()))
	for _, kind := range AllWorkKinds() {
		if _, duplicate := seen[kind]; duplicate {
			t.Fatalf("portable work kind %q is registered twice", kind)
		}
		seen[kind] = struct{}{}
		_, err := portableResponseMaximumBytesForValidatedJob(PortableJob{Kind: kind})
		if errors.Is(err, errPortableResponseMaximumKindMissing) {
			t.Fatalf("portable work kind %q has no response maximum", kind)
		}
	}
	if _, err := portableResponseMaximumBytesForValidatedJob(
		PortableJob{Kind: "unknown"},
	); !errors.Is(err, errPortableResponseMaximumKindMissing) {
		t.Fatalf("unregistered maximum error=%v", err)
	}
}

func TestPortableResponseMaximumDerivesTargetTreeGrammarBound(t *testing.T) {
	for _, fixture := range []struct {
		name     string
		rootOnly bool
		count    int
		want     int
	}{
		{name: "root files", rootOnly: true, count: 3, want: 5 + 517*3},
		{name: "nested files", count: 3, want: 5 + 817*3},
		{name: "maximum path count", count: maxTargetTreePaths, want: 5 + 817*maxTargetTreePaths},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			input := targetTreeTestInput()
			input.Constraints = TargetTreeConstraints{
				ExactPathCount: fixture.count, RootFilesOnly: fixture.rootOnly,
			}
			job, err := NewTargetTreeJob(input)
			if err != nil {
				t.Fatal(err)
			}
			assertPortableResponseMaximum(t, job, fixture.want)
		})
	}
	if got := targetTreeBranchMaximumBytes(MaxTargetTreeDepth); got != 817 {
		t.Fatalf("depth-%d target-tree branch maximum=%d want 817", MaxTargetTreeDepth, got)
	}
}

func TestPortableResponseMaximumPreservesFragmentProjectorCeilings(t *testing.T) {
	goJob, err := NewFragmentGenerationJob(FragmentGenerationInput{
		Language: "go", Dialect: "Go 1.24 function syntax",
		Signature: "func value() int", Behavior: "Return one integer value.",
	})
	if err != nil {
		t.Fatal(err)
	}
	textJob, err := NewFragmentGenerationJob(FragmentGenerationInput{
		Language: TextFragmentLanguage, Dialect: TextFragmentDialect,
		Signature: TextFragmentSignature, Behavior: "Return one proof line.",
	})
	if err != nil {
		t.Fatal(err)
	}
	typeScriptJob, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		Language: "typescript", Signature: "function value(): number",
		CurrentDeclaration: "function value(): number { return missing; }",
		RepairGuidance:     "Replace the missing expression with a numeric value.",
	})
	if err != nil {
		t.Fatal(err)
	}
	syntaxRegionJob, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		Language: "typescript", Signature: "function value(): number",
		RepairRegion: &TypeScriptFragmentRepairRegion{
			Kind: TypeScriptRepairRegionSyntaxWindow, StartLine: 1, EndLine: 1,
			Source: "return missing;",
		},
		RepairGuidance: "Return one valid replacement statement.",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name string
		job  PortableJob
		want int
	}{
		{name: "Go", job: goJob, want: MaxPortableRawCandidateBytes},
		{name: "text", job: textJob, want: MaxPortableSemanticCandidateBytes},
		{name: "whole TypeScript", job: typeScriptJob, want: MaxPortableRawCandidateBytes},
		{name: "TypeScript syntax window", job: syntaxRegionJob, want: maxTypeScriptRepairRegionBytes},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			assertPortableResponseMaximum(t, fixture.job, fixture.want)
		})
	}
}

func assertPortableResponseMaximum(t *testing.T, job PortableJob, want int) {
	t.Helper()
	got, err := PortableResponseMaximumBytesForJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("response maximum for %q=%d want %d", job.Kind, got, want)
	}
}
