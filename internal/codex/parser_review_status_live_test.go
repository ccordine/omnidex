package codex

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Uses the real production renderer and provider. Expected outcomes and fixture
// names are kept outside model context; scripted classifications are not used.
func TestReviewStatusLiveBoundary(t *testing.T) {
	path := os.Getenv("OMNI_REVIEW_STATUS_CORPUS")
	if path == "" {
		t.Skip("OMNI_REVIEW_STATUS_CORPUS is required")
	}
	provider, route := installedSieveProvider(t)
	if client, ok := provider.(*Client); ok {
		client.directory = t.TempDir()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Source, Kind string
		Role               codingagent.Role
		Task               codingagent.Task
		MinFindings        int `json:"min_findings"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(os.Getenv("OMNI_REVIEW_STATUS_EVIDENCE"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			call := codingagent.Assignment{JobID: 26, Generation: 1, Role: tc.Role, Root: t.TempDir(), Route: route, Task: &tc.Task, Response: tc.Source}
			calls := 0
			start := time.Now()
			result, runErr := codingagent.RunParserSieve(t.Context(), call, func(codingagent.NativeEvent) error { return nil }, func(ctx context.Context, leaf codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
				calls++
				prompt, err := codingagent.RenderParserPrompt(leaf)
				if err != nil {
					return codingagent.Result{}, err
				}
				background, err := codingagent.RenderParserContext(leaf)
				if err != nil {
					return codingagent.Result{}, err
				}
				started := time.Now()
				out, err := provider.Run(ctx, leaf, event)
				errText := ""
				if err != nil {
					errText = err.Error()
				}
				if saveErr := encoder.Encode(map[string]any{"case": tc.Name, "call": calls, "route": route, "prompt": prompt, "context": background, "result": out, "error": errText, "elapsed_ms": time.Since(started).Milliseconds()}); saveErr != nil {
					t.Fatal(saveErr)
				}
				return out, err
			})
			errText := ""
			if runErr != nil {
				errText = runErr.Error()
			}
			if err := encoder.Encode(map[string]any{"case": tc.Name, "summary": true, "route": route, "semantic_calls": calls, "elapsed_ms": time.Since(start).Milliseconds(), "result": result, "error": errText}); err != nil {
				t.Fatal(err)
			}
			if runErr != nil {
				t.Fatal(runErr)
			}
			var parsed codingagent.ReviewResponseResult
			if err := json.Unmarshal(result.Content, &parsed); err != nil {
				t.Fatal(err)
			}
			if string(parsed.Kind) != tc.Kind {
				t.Errorf("kind=%s want=%s", parsed.Kind, tc.Kind)
			}
			if tc.Role == codingagent.ReviewResponse {
				if parsed.Verification != tc.Source {
					t.Error("original review evidence changed")
				}
				if len(parsed.Findings) < tc.MinFindings {
					t.Errorf("findings=%d want at least %d", len(parsed.Findings), tc.MinFindings)
				}
				if tc.MinFindings == 0 && len(parsed.Findings) != 0 {
					t.Errorf("findings=%d want none", len(parsed.Findings))
				}
			}
			t.Logf("route=%+v semantic_calls=%d elapsed=%s kind=%s findings=%d", route, calls, time.Since(start), parsed.Kind, len(parsed.Findings))
		})
	}
}
