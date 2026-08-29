package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSemanticDomainPackagesDoNotOwnProviderTransports(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, relative := range []string{
		"internal/specialists",
		"internal/specialistworkflow",
		"internal/contextcompiler",
		"internal/webresearch",
		"internal/roleplay",
	} {
		walkProductionSource(t, filepath.Join(root, relative), func(path, source string) {
			for _, forbidden := range []string{
				`github.com/gryph/omnidex/internal/llm`,
				`github.com/gryph/omnidex/internal/ollama`,
				`github.com/gryph/omnidex/internal/openai`,
				`github.com/gryph/omnidex/internal/googleai`,
				`github.com/gryph/omnidex/internal/huggingface`,
				"GeneratePreparedExact(",
				`/api/generate`,
				`/api/chat`,
				`response_format`,
				`json_schema`,
			} {
				if strings.Contains(source, forbidden) {
					t.Errorf("semantic domain source %s owns forbidden provider contract %q", path, forbidden)
				}
			}
		})
	}
}

func TestOllamaPrewarmLoadsWithoutAnInferencePrompt(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "cmd", "cli", "ollama_prewarm.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		`baseURL+"/api/generate"`,
		`response.Response != ""`,
		`response.PromptEvalCount != 0`,
		`response.EvalCount != 0`,
	} {
		if !strings.Contains(source, required) {
			t.Errorf("Ollama prewarm omitted non-inference guard %q", required)
		}
	}
	for _, forbidden := range []string{
		"MinimalGeneratePrompt", "ollamaPrewarmMessage", "NumPredict", "Temperature",
		`baseURL+"/api/chat"`,
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Ollama prewarm retains ceremonial inference authority %q", forbidden)
		}
	}
}

func TestPortableRendererReturnsOnlyRawPromptAndError(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "assemblyline", "portable_job_render.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var renderer *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "RenderPortableJob" {
			renderer = function
			break
		}
	}
	if renderer == nil || renderer.Type.Results == nil {
		t.Fatal("RenderPortableJob declaration is missing")
	}
	results := renderer.Type.Results.List
	if len(results) != 2 {
		t.Fatalf("RenderPortableJob returns %d result fields, want raw prompt and error", len(results))
	}
	for index, expected := range []string{"string", "error"} {
		identifier, ok := results[index].Type.(*ast.Ident)
		if !ok || identifier.Name != expected || len(results[index].Names) != 0 {
			t.Fatalf("RenderPortableJob result %d is %#v, want unnamed %s", index, results[index], expected)
		}
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	walkProductionSource(t, filepath.Join(root, "internal", "assemblyline"), func(path, source string) {
		if strings.Contains(source, "TargetTreeResponseSchema") {
			t.Errorf("assemblyline source %s retained the removed target-tree schema channel", path)
		}
		if strings.HasPrefix(filepath.Base(path), "portable_job_render") &&
			strings.Contains(source, "map[string]any") {
			t.Errorf("portable renderer source %s exposes a map response channel", path)
		}
	})
}

func TestProductionHasNoWholeRelationalIntentModelJSONDecoder(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	retired := filepath.Join(root, "internal", "datasource", "relational_intent_decode.go")
	if _, err := os.Stat(retired); !os.IsNotExist(err) {
		t.Fatalf("retired whole relational-intent JSON decoder still exists: %v", err)
	}
	walkProductionSource(t, root, func(path, source string) {
		if strings.Contains(source, "DecodeRelationalIntent") {
			t.Errorf("production source %s retains whole relational-intent JSON decoding authority", path)
		}
	})
}

func TestRetiredBrowserInferenceProductionSurfaceIsAbsent(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, relative := range []string{
		"internal/browserinference/context_relevance_broker.go",
		"internal/api/browser_context_relevance_http.go",
		"internal/api/web/src/controllers/browser_inference_controller.ts",
		"internal/api/web/src/lib/browser_context_relevance_runtime.ts",
		"internal/api/web/src/workers/browser_inference_worker.ts",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Errorf("retired browser inference source %s still exists: %v", relative, err)
		}
	}
	for _, relative := range []string{
		"cmd/core/main.go",
		"internal/api/server.go",
		"internal/worker/engine.go",
		"internal/worker/objective_context_sieve_stations.go",
		"internal/api/web/package.json",
		"internal/api/web/src/main.ts",
	} {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"browserContextRelevance", "BrowserContextRelevance", "browser-inference",
			"@mlc-ai/web-llm",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("production source %s retains browser inference authority %q", relative, forbidden)
			}
		}
	}
}

func TestProductionSourceContainsNoRetiredRawTransportIdentity(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", "..", "internal"))
	forbidden := []string{
		"omnidex.ollama-raw-text-generate-request.v1",
		"ollama-0.24.0-qwen35-gpt2-boundary-v1",
		"ExactPreparedProtocolRawTextV1",
	}
	walkProductionSource(t, root, func(path, source string) {
		for _, identity := range forbidden {
			if strings.Contains(source, identity) {
				t.Errorf("production source %s retains retired raw transport identity %q", path, identity)
			}
		}
	})
}

func TestCurrentStationTransportHasNoResponseSchemaAuthority(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, relative := range []string{
		"internal/queue",
		"internal/worker",
	} {
		walkProductionSource(t, filepath.Join(root, relative), func(path, source string) {
			for _, forbidden := range []string{
				"ResponseSchema", "response_schema", "canonicalStationGapSchema",
			} {
				if strings.Contains(source, forbidden) {
					t.Errorf("current station source %s retains retired authority %q", path, forbidden)
				}
			}
		})
	}
}

func structuredModelOutputTokens() []string {
	return []string{
		`json:"format`, `json:"response_format`, "response_format",
		"json_schema", "ResponseSchema", "structured_outputs",
	}
}

func walkProductionSource(t *testing.T, root string, inspect func(string, string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.HasSuffix(entry.Name(), "_test.go") ||
			!strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		inspect(path, string(raw))
		return nil
	})
	if err != nil {
		t.Fatalf("scan production source under %s: %v", root, err)
	}
}
