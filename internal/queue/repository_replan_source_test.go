package queue

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestReplanSourceHasOneGenerationPathAndNoResetFallback(t *testing.T) {
	paths := []string{
		"repository_replan.go", "repository_replan_commit.go",
		"job_generation_replan.go", "job_generation_store.go",
	}
	var source strings.Builder
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source.Write(raw)
		source.WriteByte('\n')
	}
	text := source.String()
	for _, forbidden := range []string{
		"INSERT INTO step_contexts",
		`"replan_feedback"`,
		`"user_feedback"`,
		"output = NULL",
		"started_at = NULL",
		"action IN ('v3_coding', 'v3_subtask', 'v3_planning', 'plan')",
		"rejectUnresolvedRepositoryMutationsTx(",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("replan source contains forbidden reset/fallback path %q", forbidden)
		}
	}
	updateToParameter := regexp.MustCompile(`(?is)UPDATE\s+job_steps\s+SET\s+status\s*=\s*\$[0-9]+`)
	if updateToParameter.MatchString(text) {
		t.Fatal("replan must not reset existing steps to a parameterized status")
	}
	for _, required := range []string{
		"INSERT INTO job_generations",
		"superseded_at_generation",
		"current_generation",
		"feedback_sha256",
		"rejectUnresolvedWorkspaceMutationsTx(",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("replan source is missing generation authority %q", required)
		}
	}
}

func TestLegacyRepositoryMutationLifecycleChecksHaveNoQueueProductionCallers(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch function := call.Fun.(type) {
			case *ast.Ident:
				if function.Name == "rejectUnresolvedRepositoryMutationsTx" {
					t.Errorf("queue production source %s calls legacy lifecycle guard %s", name, function.Name)
				}
			case *ast.SelectorExpr:
				if function.Sel.Name == "UnresolvedRepositoryMutation" {
					t.Errorf("queue production source %s calls legacy unresolved-mutation loader", name)
				}
			}
			return true
		})
	}
}
