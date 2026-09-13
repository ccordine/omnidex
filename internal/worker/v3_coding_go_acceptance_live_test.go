package worker

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
)

func TestLiveGoAcceptanceProducesExecutableAssertions(t *testing.T) {
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
		name, behavior, implementation string
		kind                           assemblyline.ApplicationResultValueKind
	}{
		{"literal output", `Print the text "ready" to standard output.`, `return "ready"`, assemblyline.ApplicationResultText},
		{"text enclosure", "Output the first supplied argument surrounded by square brackets.", `return "[" + arguments[0] + "]"`, assemblyline.ApplicationResultText},
		{"computed product", "Print the product of the integers supplied as two command-line arguments, in decimal.", `left, _ := strconv.Atoi(arguments[0])
right, _ := strconv.Atoi(arguments[1])
return left * right`, assemblyline.ApplicationResultInteger},
		{"computed reversal", "Print the characters of the text supplied as the first command-line argument in reverse order.", `letters := []rune(arguments[0])
for left, right := 0, len(letters)-1; left < right; left, right = left+1, right-1 { letters[left], letters[right] = letters[right], letters[left] }
return string(letters)`, assemblyline.ApplicationResultText},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			program, _, _, _ := goAcceptanceFixtureForBehavior(t, fixture.behavior, fixture.implementation, fixture.kind)
			for _, role := range []assemblyline.SourceBlockRole{assemblyline.SourceBlockTaskExample, assemblyline.SourceBlockTaskVerification} {
				ref := goAcceptanceFixtureBlock(t, program, role)
				input, err := directCodingGoFragmentInput(&program, ref)
				if err != nil {
					t.Fatal(err)
				}
				declaration, err := runDirectCodingLanguageFragmentWorker(liveSourceBodyRuntime(t, client, model, contextTokens), model, directCodingLanguageGenerationJob{
					Subject: ref.Block.ID, Input: input, Validate: validateDirectCodingGoFragment,
				})
				if err != nil {
					t.Fatal(err)
				}
				program.Generated[ref.Block.ID] = declaration
			}
			runLiveGoAcceptanceFixture(t, program, false)
			program.Generated["feature.001"] = goWrongValueDeclaration(t, program)
			runLiveGoAcceptanceFixture(t, program, true)
		})
	}
}

func runLiveGoAcceptanceFixture(t *testing.T, program directCodingProgram, wantFailure bool) {
	t.Helper()
	sources, err := composeDirectCodingSourceProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.24.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if err := os.WriteFile(filepath.Join(root, source.Path), []byte(source.Source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.CommandContext(t.Context(), "go", "test", "-count=1", "./...")
	command.Dir = root
	command.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOTOOLCHAIN=local")
	output, err := command.CombinedOutput()
	if (err != nil) != wantFailure || (wantFailure && !strings.Contains(string(output), "--- FAIL: TestFeature001")) {
		t.Fatalf("test error=%v; wantFailure=%t\n%s", err, wantFailure, output)
	}
}
