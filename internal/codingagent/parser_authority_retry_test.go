package codingagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type authorityRetryProvider struct {
	executionScopeProvider
	failRole Role
	failed   bool
	effects  map[Role]int
}

func (p *authorityRetryProvider) Run(ctx context.Context, call Assignment, event func(NativeEvent) error) (Result, error) {
	if call.Role == Execute || call.Role == Review {
		p.effects[call.Role]++
		if call.Role == Execute {
			if err := os.WriteFile(filepath.Join(call.Root, "output.txt"), []byte("retained native effect\n"), 0600); err != nil {
				return Result{}, err
			}
		}
	}
	if call.Role == p.failRole && !p.failed {
		p.failed = true
		return Result{}, fmt.Errorf("synthetic unresolved parser interpretation")
	}
	return p.executionScopeProvider.Run(ctx, call, event)
}

func TestRoleScopedParserRetryPreservesCompletedEffects(t *testing.T) {
	for _, role := range []Role{ExecutionResponse, ReviewResponse} {
		t.Run(string(role), func(t *testing.T) {
			task := Task{ID: "scope-task", Goal: "Write and verify output.txt. Omnidex independently reviews after execution.", WriteFiles: []string{"output.txt"}}
			p := &authorityRetryProvider{executionScopeProvider: executionScopeProvider{t: t, task: task, response: "Implementation and required checks complete. Independent review follows this response.", kind: "done"}, failRole: role, effects: map[Role]int{}}
			route := Route{Provider: "fixture", Model: "fixture", Effort: "high", ServiceTier: "default"}
			store := &durableTaskEvidence{}
			controller := Controller{JobID: 1, Generation: 1, Root: t.TempDir(), Evidence: store, Environment: durableTaskEnvironment{}, Providers: map[string]Provider{"fixture": p}, Configuration: Configuration{Mode: Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route}}
			var failure *ParserFailure
			if err := controller.RunTask(t.Context(), task); !errors.As(err, &failure) {
				t.Fatalf("parser failure lost: %v", err)
			}
			if err := controller.RunTask(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			if err := controller.RunTask(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			if p.effects[Execute] != 1 || p.effects[Review] != 1 {
				t.Fatalf("completed native effects repeated: %v", p.effects)
			}
			data, err := os.ReadFile(filepath.Join(controller.Root, "output.txt"))
			if err != nil || string(data) != "retained native effect\n" {
				t.Fatalf("retained effect lost: %q %v", data, err)
			}
			for _, entry := range store.entries {
				if entry.Phase == "execution_response" && entry.Notes[0] != p.response {
					t.Fatal("original response changed")
				}
			}
		})
	}
}
