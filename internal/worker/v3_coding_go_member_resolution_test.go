package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoReceiverMembersAreCompiledWithoutSemanticCorrection(t *testing.T) {
	for _, fixture := range []struct {
		name, body, expected string
		kind                 assemblyline.ApplicationResultValueKind
	}{
		{"text builder", `var value strings.Builder
// value.WriteString(" ") // An unused expression remains commentary.
value.WriteString(arguments[0]); return value.String()`, `return "sample"`, assemblyline.ApplicationResultText},
		{"date component", `value := time.Date(2020, time.January, 2, 0, 0, 0, 0, time.UTC)
// value.Year() // The actual value is returned below.
return value.Year()`, `return 2020`, assemblyline.ApplicationResultInteger},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			program, _, expected, _ := goAcceptanceFixtureForBehavior(t, "Return the locally computed value.", fixture.body, fixture.kind)
			implementation := goAcceptanceFixtureBlock(t, program, assemblyline.SourceBlockTaskImplementation)
			input, err := directCodingGoFragmentInput(&program, implementation)
			if err != nil {
				t.Fatal(err)
			}
			declaration, _, correction, err := validateDirectCodingLanguageBody(assemblyline.ArtifactIdentityProvenance{}, directCodingLanguageGenerationJob{Input: input, Validate: validateDirectCodingGoFragment}, fixture.body)
			if err != nil {
				t.Fatalf("valid receiver member created semantic correction: %v", err)
			}
			if correction != nil {
				t.Fatal("valid source created correction work")
			}
			program.Generated[implementation.Block.ID] = declaration
			program.Generated[expected.Block.ID] = expected.Block.Signature + " { " + fixture.expected + " }"
			example := goAcceptanceFixtureBlock(t, program, assemblyline.SourceBlockTaskExample)
			program.Generated[example.Block.ID] = example.Block.Signature + ` { return []string{"sample"} }`
			runLiveGoAcceptanceFixture(t, program, false)
			if _, err := validateDirectCodingGoFragment(input, `return unavailable.Value()`); err == nil {
				t.Fatal("unknown receiver acquired lexical authority")
			}
		})
	}
}
