package worker

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	liveQwenRawFragmentModelEnv = "OMNIDEX_TEST_QWEN_RAW_FRAGMENT_MODEL"
	liveQwenRawFragmentModel    = "qwen3.5:9b-q4_K_M"
	liveQwenRawFragmentScope    = "live-qwen-raw-fragment-route-v1"
)

func TestLiveQwenRawFragmentRouteQualification(t *testing.T) {
	modelName := strings.TrimSpace(os.Getenv(liveQwenRawFragmentModelEnv))
	if modelName == "" {
		t.Skip(liveQwenRawFragmentModelEnv + " is not set")
	}
	if modelName != liveQwenRawFragmentModel {
		t.Fatalf("%s=%q want %q", liveQwenRawFragmentModelEnv, modelName, liveQwenRawFragmentModel)
	}
	baseURL := requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_URL")
	requireLiveCodingQualificationEnv(t, "OMNI_TEST_DATABASE_URL")
	contextTokens, err := strconv.Atoi(requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_CONTEXT"))
	if err != nil || contextTokens <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	_, repository, pool := openRepositoryTestDatabase(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Minute)
	defer cancel()
	provider := newLiveRawFragmentStationClient(
		ollama.New(baseURL, modelName, "", 10*time.Minute, contextTokens),
	)
	opts := validWorkerOptions()
	opts.InferenceContextTokens = contextTokens
	service, err := New(repository, provider, startupTestLLM{}, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	jobRecord, err := repository.EnqueueJob(
		ctx, liveQwenRawFragmentScope+"-"+time.Now().UTC().Format("20060102150405.000000000"),
		model.PipelineCoding, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, liveQwenRawFragmentScope+"-worker")
	if err != nil || claim == nil || claim.Job.ID != jobRecord.ID {
		t.Fatalf("raw fragment live claim=%+v error=%v", claim, err)
	}
	productionRuntime := portableWorkerRuntimeWithContext(
		&nativeRuntimeV3{svc: service, ctx: ctx, claim: claim, action: "live_raw_fragment"},
		liveQwenRawFragmentScope, ctx,
	)

	t.Run("tsx generation", func(t *testing.T) {
		block := assemblyline.SourceBlock{
			ID:        "fixture.presentation",
			Signature: "function PresentLabel({ label }: LabelProps): ReactElement",
			Contract:  "Return exactly one <output> JSX element whose only rendered child is label.",
			API:       "function PresentLabel({ label }: LabelProps): ReactElement",
			Globals:   []string{"ReactElement"},
			Policy: assemblyline.SourceFunctionPolicy{
				RequiredElementNames: []string{"output"},
			},
		}
		fragmentJob := directCodingTypeScriptFragmentJob{
			block: block, dialect: "TypeScript 5.9.3 TSX function syntax", tsx: true,
			available: "interface LabelProps { readonly label: string }",
		}
		portable, err := newDirectCodingTypeScriptPortableJob(fragmentJob)
		if err != nil {
			t.Fatal(err)
		}
		start := provider.callCount()
		source, err := runDirectCodingTypeScriptFragmentWorker(
			productionRuntime, modelName, fragmentJob,
		)
		if err != nil {
			t.Fatal(err)
		}
		requireLiveTSXFragmentTypechecks(t, source)
		assertLiveProductionRawFragment(
			t, pool, jobRecord.ID, portable, source,
			queue.StationGapProjectionTypeScriptFunction, provider.callsFrom(start),
		)
	})

	t.Run("go generation", func(t *testing.T) {
		input := assemblyline.FragmentGenerationInput{
			Language: "go", Dialect: "Go 1.24 function syntax",
			Signature: "func ClampFloor(value int, floor int) int",
			Behavior:  "Return floor when value is less than floor; otherwise return value.",
		}
		portable, err := assemblyline.NewFragmentGenerationJob(input)
		if err != nil {
			t.Fatal(err)
		}
		start := provider.callCount()
		source, err := runDirectCodingGoFragmentGenerationWorker(
			productionRuntime, modelName, directCodingGoGenerationJob{
				Subject: "fixture.numeric_bounds", Input: input,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		requireLiveGoFragmentTypechecks(t, source)
		assertLiveProductionRawFragment(
			t, pool, jobRecord.ID, portable, source,
			queue.StationGapProjectionSourceDeclaration, provider.callsFrom(start),
		)
	})

	t.Run("go correction", func(t *testing.T) {
		const signature = "func Toggle(enabled bool) bool"
		const current = "func Toggle(enabled bool) bool { return enabled }"
		const instruction = "Return the logical negation of enabled."
		contract := gofragment.Contract{Signature: signature, Current: current}
		portable, err := assemblyline.NewSourceProjectedFragmentCorrectionJob(
			assemblyline.FragmentCorrectionInput{
				CurrentDeclaration: current, RepairGuidance: instruction,
			},
			"go",
		)
		if err != nil {
			t.Fatal(err)
		}
		start := provider.callCount()
		source, err := runDirectCodingLanguageCorrection(
			productionRuntime, modelName, "fixture.boolean_toggle", current,
			instruction, "go",
			func(candidate string) (string, error) {
				return gofragment.ParseFunction(contract, candidate)
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		requireLiveGoFragmentTypechecks(t, source)
		assertLiveProductionRawFragment(
			t, pool, jobRecord.ID, portable, source,
			queue.StationGapProjectionSourceDeclaration, provider.callsFrom(start),
		)
	})

	t.Run("tsx guided repair", func(t *testing.T) {
		runLiveQwenGuidedTSXRepairQualification(
			t, productionRuntime, modelName, provider, pool, jobRecord.ID,
		)
	})
}

func requireLiveGoFragmentTypechecks(t *testing.T, source string) {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "", "package qualification\n\n"+source, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse qualified Go declaration: %v", err)
	}
	if _, err := (&types.Config{}).Check("qualification", fileSet, []*ast.File{file}, nil); err != nil {
		t.Fatalf("typecheck qualified Go declaration: %v", err)
	}
}

func requireLiveTSXFragmentTypechecks(t *testing.T, source string) {
	t.Helper()
	compiler := filepath.Clean(filepath.Join("..", "api", "web", "node_modules", ".bin", "tsc"))
	if _, err := os.Stat(compiler); err != nil {
		t.Fatalf("live TSX qualification requires %s: %v", compiler, err)
	}
	qualified := strings.Join([]string{
		"declare namespace JSX { interface Element {} interface IntrinsicElements { output: { children?: unknown } } }",
		"type ReactElement = JSX.Element;",
		"interface LabelProps { readonly label: string }",
		source,
	}, "\n")
	path := filepath.Join(t.TempDir(), "qualification.tsx")
	if err := os.WriteFile(path, []byte(qualified), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(
		t.Context(), compiler, "--noEmit", "--strict", "--jsx", "preserve",
		"--target", "ES2022", "--module", "ESNext", "--skipLibCheck", path,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("typecheck qualified TSX declaration: %v\n%s", err, output)
	}
}
