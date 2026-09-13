package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoInputChannelsOwnSignaturesAndExamples(t *testing.T) {
	for _, fixture := range []struct {
		name, behavior, body, example, parameter string
		channel                                  assemblyline.ApplicationInputSource
	}{
		{"arguments", "Return the first argument surrounded by brackets.", `return "[" + arguments[0] + "]"`, `return []string{"sample"}`, "arguments []string", assemblyline.ApplicationInputArguments},
		{"standard input", "Return the standard-input text in uppercase.", `return strings.ToUpper(text)`, `return "sample"`, "text string", assemblyline.ApplicationInputStandardInput},
		{"no input", "Return the literal text ready.", `return "ready"`, "", "", assemblyline.ApplicationInputNone},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			program, _, expected, input := goAcceptanceFixtureWithInput(t, fixture.behavior, fixture.body, assemblyline.ApplicationResultText, fixture.channel)
			if input.Signature != "func ExpectedFeature001("+fixture.parameter+") string" || len(input.Capabilities) != 0 {
				t.Fatalf("unexpected input boundary: %+v", input)
			}
			program.Generated[expected.Block.ID] = expected.Block.Signature + " { " + fixture.body + " }"
			if fixture.channel == assemblyline.ApplicationInputNone {
				for _, document := range program.Source.Documents {
					for _, block := range document.Blocks {
						if block.Role == assemblyline.SourceBlockTaskExample {
							t.Fatal("empty input manufactured inference")
						}
					}
				}
			} else {
				example := goAcceptanceFixtureBlock(t, program, assemblyline.SourceBlockTaskExample)
				program.Generated[example.Block.ID] = example.Block.Signature + " { " + fixture.example + " }"
			}
			job, err := assemblyline.NewFragmentGenerationJob(input)
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(prompt, "TaskInput") || strings.Contains(prompt, "StandardInput") {
				t.Fatalf("unrequested input declaration: %s", prompt)
			}
			runLiveGoAcceptanceFixture(t, program, false)
		})
	}
}
