package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPHPStageRepairMapsSyntaxAndAcceptanceToExactMutableOwners(t *testing.T) {
	t.Parallel()
	program, documents := phpRepairMappingFixture()
	tests := []struct {
		name      string
		command   testCommand
		output    string
		wantBlock string
		wantText  string
	}{
		{
			name: "syntax declaration",
			command: testCommand{Purpose: verificationSyntax, Args: []string{
				"compose", "run", "app", "php", "-l", "src/Feature101.php",
			}},
			output:    "PHP Parse error: syntax error, unexpected token \";\" in /srv/app/src/Feature101.php on line 6\nErrors parsing src/Feature101.php",
			wantBlock: "feature.101", wantText: "DECLARATION_LOCATION: line 2",
		},
		{
			name: "acceptance implementation owner",
			command: testCommand{Purpose: verificationTest, Args: []string{
				"compose", "run", "app", "php", "tests/Feature702Test.php",
			}},
			output:    "PHP Fatal error: Uncaught RuntimeException: returned result differs from the asserted value in /srv/app/src/Runtime.php:263\nStack trace:\n#0 /srv/app/tests/Feature702Test.php(12)",
			wantBlock: "feature.702", wantText: "returned result differs",
		},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			mapped, ok, err := mapDirectCodingPHPStageFailure(
				program, documents, testCase.command, testCase.output,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !ok || mapped.Target.Block.ID != testCase.wantBlock ||
				!strings.Contains(mapped.Diagnostic, testCase.wantText) {
				t.Fatalf("mapped=%+v ok=%t", mapped, ok)
			}
			for _, forbidden := range []string{"src/", "tests/", "/srv/", "compose", "Feature702Test"} {
				if strings.Contains(mapped.Diagnostic, forbidden) {
					t.Fatalf("diagnostic leaked %q: %s", forbidden, mapped.Diagnostic)
				}
			}
		})
	}
}

func TestPHPStageRepairRejectsUnownedMalformedAndVerificationSyntaxFailures(t *testing.T) {
	t.Parallel()
	program, documents := phpRepairMappingFixture()
	cases := []struct {
		name    string
		command testCommand
		output  string
		wantErr bool
	}{
		{
			name: "unknown source", command: testCommand{Purpose: verificationSyntax, Args: []string{"php", "-l", "src/Unknown.php"}},
			output: "PHP Parse error: syntax error in /srv/app/src/Unknown.php on line 2",
		},
		{
			name: "malformed diagnostic", command: testCommand{Purpose: verificationSyntax, Args: []string{"php", "-l", "src/Feature101.php"}},
			output: "lint failed without a location",
		},
		{
			name: "verification source is immutable context", command: testCommand{Purpose: verificationSyntax, Args: []string{"php", "-l", "tests/Feature702Test.php"}},
			output: "PHP Parse error: syntax error, unexpected token \";\" in /srv/app/tests/Feature702Test.php on line 6", wantErr: true,
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, ok, err := mapDirectCodingPHPStageFailure(program, documents, testCase.command, testCase.output)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected explicit immutable-verification rejection")
				}
				return
			}
			if err != nil || ok {
				t.Fatalf("ok=%t err=%v", ok, err)
			}
		})
	}
}

func phpRepairMappingFixture() (directCodingProgram, []assemblyline.ComposedSourceDocument) {
	implementation101 := assemblyline.SourceBlock{
		ID: "feature.101", Signature: "function feature101(array $values): array",
		Contract: "Transform values.", API: "function feature101(array $values): array",
		TaskID: "task_101", Role: assemblyline.SourceBlockTaskImplementation,
	}
	implementation702 := assemblyline.SourceBlock{
		ID: "feature.702", Signature: "function feature702(array $values): array",
		Contract: "Transform values.", API: "function feature702(array $values): array",
		TaskID: "task_702", Role: assemblyline.SourceBlockTaskImplementation,
	}
	verification702 := assemblyline.SourceBlock{
		ID: "acceptance.702", Signature: "function verifyFeature702(): void",
		Contract: "Verify the result.", API: "function verifyFeature702(): void",
		DependsOn: []string{"feature.702"}, TaskID: "task_702",
		Role: assemblyline.SourceBlockTaskVerification,
	}
	program := directCodingProgram{Source: assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
		{ID: "implementation_101", Path: "src/Feature101.php", AdapterID: "php", Blocks: []assemblyline.SourceBlock{implementation101}},
		{ID: "implementation_702", Path: "src/Feature702.php", AdapterID: "php", Blocks: []assemblyline.SourceBlock{implementation702}},
		{ID: "verification_702", Path: "tests/Feature702Test.php", AdapterID: "php", Blocks: []assemblyline.SourceBlock{verification702}},
	}}}
	documents := []assemblyline.ComposedSourceDocument{
		{ID: "implementation_101", Path: "src/Feature101.php", Spans: map[string]assemblyline.SourceSpan{"feature.101": {StartLine: 5, EndLine: 8}}},
		{ID: "implementation_702", Path: "src/Feature702.php", Spans: map[string]assemblyline.SourceSpan{"feature.702": {StartLine: 5, EndLine: 8}}},
		{ID: "verification_702", Path: "tests/Feature702Test.php", Spans: map[string]assemblyline.SourceSpan{"acceptance.702": {StartLine: 5, EndLine: 9}}},
	}
	return program, documents
}
