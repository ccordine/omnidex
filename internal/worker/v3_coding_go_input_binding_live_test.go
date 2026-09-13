package worker

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
)

func TestLiveGoSourceUsesTheRequestedInputBoundary(t *testing.T) {
	model := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_GO_TEST_MODEL"))
	if model == "" {
		t.Skip("OMNIDEX_TEST_GO_TEST_MODEL is not set")
	}
	endpoint := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_OLLAMA_URL"))
	contextTokens, err := strconv.Atoi(os.Getenv("OMNIDEX_TEST_OLLAMA_CONTEXT"))
	if endpoint == "" || err != nil || contextTokens <= 0 {
		t.Fatal("a live endpoint and positive context-token limit are required")
	}
	client := ollama.New(endpoint, model, "", llm.MaximumModelRequestDuration)
	for _, fixture := range []struct {
		name, behavior, example, expected, initial string
		kind                                       assemblyline.ApplicationResultValueKind
	}{
		{"numeric arguments", "The software prints the product of the two numbers supplied as its arguments.",
			`return []string{"6", "7"}`, `return 42`, `return 0`, assemblyline.ApplicationResultInteger},
		{"text argument", "The software prints the text supplied as its argument unchanged.",
			`return []string{"bound-text"}`, `return "bound-text"`, `return ""`, assemblyline.ApplicationResultText},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			program, _, verification, _ := goAcceptanceFixtureForBehavior(t, fixture.behavior, fixture.initial, fixture.kind)
			var implementation assemblyline.SourceBlockRef
			for _, document := range program.Source.Documents {
				for _, block := range document.Blocks {
					if block.Role == assemblyline.SourceBlockTaskImplementation {
						implementation = assemblyline.SourceBlockRef{Document: document, Block: block}
					}
				}
			}
			input, err := directCodingGoFragmentInput(&program, implementation)
			if err != nil {
				t.Fatal(err)
			}
			declaration, err := runDirectCodingLanguageFragmentWorker(liveSourceBodyRuntime(t, client, model, contextTokens), model, directCodingLanguageGenerationJob{
				Subject: implementation.Block.ID, Input: input, Validate: validateDirectCodingGoFragment,
			})
			if err != nil {
				t.Fatal(err)
			}
			program.Generated[implementation.Block.ID] = declaration
			example := goAcceptanceFixtureBlock(t, program, assemblyline.SourceBlockTaskExample)
			program.Generated[example.Block.ID] = example.Block.Signature + " {\n" + fixture.example + "\n}"
			program.Generated[verification.Block.ID] = verification.Block.Signature + " {\n" + fixture.expected + "\n}"
			runLiveGoAcceptanceFixture(t, program, false)
		})
	}
}
