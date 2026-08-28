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

func TestIntentionalNonStationInferenceSurfacesRemainRawTextOnly(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	checks := map[string]struct {
		required  []string
		forbidden []string
	}{
		"cmd/cli/screen_local.go": {
			required: []string{
				`Response string ` + "`json:\"response\"`",
				"result := normalizeScreenText(parsed.Response)",
			},
			forbidden: structuredModelOutputTokens(),
		},
		"cmd/cli/ollama_prewarm.go": {
			required: []string{
				`Content string ` + "`json:\"content\"`",
				`strings.TrimSpace(response.Message.Content) == ""`,
			},
			forbidden: structuredModelOutputTokens(),
		},
		"internal/api/web/src/lib/browser_context_relevance_protocol.ts": {
			required: []string{
				"raw_result?: string", "raw_result: rawResult",
				"ChatCompletionRequestNonStreaming",
			},
			forbidden: append(structuredModelOutputTokens(),
				"JSON.parse(rawResult)", "JSON.parse(content)"),
		},
	}
	for relative, check := range checks {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		source := string(raw)
		for _, required := range check.required {
			if !strings.Contains(source, required) {
				t.Errorf("raw inference surface %s omitted %q", relative, required)
			}
		}
		for _, forbidden := range check.forbidden {
			if strings.Contains(source, forbidden) {
				t.Errorf("raw inference surface %s contains structured output contract %q", relative, forbidden)
			}
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

func TestBrowserInferencePortableBoundaryHasNoStructuredResponseChannel(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(
		"..", "browserinference", "context_relevance_broker.go",
	))
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"prompt, err := assemblyline.RenderPortableJob(job)",
		"DecodeContextRelevanceSelectionDecision(",
		"request.input, submission.RawResult",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("browser raw relevance boundary omitted %q", required)
		}
	}
	for _, forbidden := range []string{"responseSchema", "ResponseSchema", "json_schema", "map[string]any"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("browser raw relevance boundary retained structured channel %q", forbidden)
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
