package odn

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func BuildExecutionPlan(message, workspacePath string) (ExecutionPlan, bool) {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if normalized == "" {
		return ExecutionPlan{}, false
	}

	hasCreateVerb := strings.Contains(normalized, "make") || strings.Contains(normalized, "create") || strings.Contains(normalized, "build") || strings.Contains(normalized, "scaffold")
	hasProject := strings.Contains(normalized, "project")
	hasGo := strings.Contains(normalized, "go") || strings.Contains(normalized, "golang")
	hasHTML := strings.Contains(normalized, "html")

	if !(hasCreateVerb && hasProject && hasGo && hasHTML) {
		return ExecutionPlan{}, false
	}

	return BuildGoHTMLScaffoldPlan(workspacePath), true
}

func BuildGoHTMLScaffoldPlan(workspacePath string) ExecutionPlan {
	targetDir := filepath.Join(workspacePath, defaultProjectFolderName)
	return ExecutionPlan{
		Name:    "scaffold_go_html_test_project",
		Summary: fmt.Sprintf("Create a Go + HTML test project in %s", targetDir),
		Actions: []PlannedAction{
			{
				ID:          "a1",
				Kind:        "mkdir",
				Description: "Create project root directory",
				Path:        targetDir,
				RiskTier:    1,
			},
			{
				ID:          "a2",
				Kind:        "write",
				Description: "Write go.mod",
				Path:        filepath.Join(targetDir, "go.mod"),
				Content:     scaffoldGoMod(),
				RiskTier:    1,
			},
			{
				ID:          "a3",
				Kind:        "mkdir",
				Description: "Create cmd/web directory",
				Path:        filepath.Join(targetDir, "cmd", "web"),
				RiskTier:    1,
			},
			{
				ID:          "a4",
				Kind:        "write",
				Description: "Write cmd/web/main.go",
				Path:        filepath.Join(targetDir, "cmd", "web", "main.go"),
				Content:     scaffoldMainGo(),
				RiskTier:    1,
			},
			{
				ID:          "a5",
				Kind:        "mkdir",
				Description: "Create web directory",
				Path:        filepath.Join(targetDir, "web"),
				RiskTier:    1,
			},
			{
				ID:          "a6",
				Kind:        "write",
				Description: "Write web/index.html",
				Path:        filepath.Join(targetDir, "web", "index.html"),
				Content:     scaffoldHTML(),
				RiskTier:    1,
			},
			{
				ID:          "a7",
				Kind:        "write",
				Description: "Write README.md",
				Path:        filepath.Join(targetDir, "README.md"),
				Content:     scaffoldREADME(),
				RiskTier:    1,
			},
		},
	}
}

func ExecutePlan(plan ExecutionPlan, mode PermissionMode, in io.Reader, out io.Writer, workspacePath string, nextEventID func() string) ([]Event, error) {
	events := make([]Event, 0, len(plan.Actions)+4)
	events = append(events, Event{
		ID:      nextEventID(),
		Type:    "plan_generated",
		Summary: plan.Summary,
		Details: map[string]string{
			"plan_name":      plan.Name,
			"action_count":   fmt.Sprintf("%d", len(plan.Actions)),
			"workspace_path": workspacePath,
		},
		CreatedAt: nowUTC(),
	})

	writeActionCount := 0
	for _, action := range plan.Actions {
		if action.Kind == "write" || action.Kind == "mkdir" {
			writeActionCount++
		}
	}

	if mode == PermissionAsk && writeActionCount > 0 {
		approved, err := PromptYesNo(in, out, fmt.Sprintf("This plan includes %d write actions. Approve? [y/N]: ", writeActionCount))
		if err != nil {
			return events, err
		}
		if !approved {
			events = append(events, Event{
				ID:        nextEventID(),
				Type:      "permission_denied",
				Summary:   "User denied write permission",
				Details:   map[string]string{"mode": string(mode)},
				CreatedAt: nowUTC(),
			})
			return events, nil
		}
		events = append(events, Event{
			ID:        nextEventID(),
			Type:      "permission_granted",
			Summary:   "User granted write permission",
			Details:   map[string]string{"mode": string(mode)},
			CreatedAt: nowUTC(),
		})
	} else {
		events = append(events, Event{
			ID:        nextEventID(),
			Type:      "permission_not_required",
			Summary:   "No interactive approval required",
			Details:   map[string]string{"mode": string(mode)},
			CreatedAt: nowUTC(),
		})
	}

	for _, action := range plan.Actions {
		if !isWithinWorkspace(workspacePath, action.Path) {
			events = append(events, Event{
				ID:        nextEventID(),
				Type:      "policy_blocked",
				Summary:   "Action path escaped workspace boundary",
				Details:   map[string]string{"path": action.Path},
				CreatedAt: nowUTC(),
			})
			return events, fmt.Errorf("action %s path escapes workspace: %s", action.ID, action.Path)
		}

		if err := applyAction(action); err != nil {
			events = append(events, Event{
				ID:        nextEventID(),
				Type:      "action_failed",
				Summary:   "Action failed",
				Details:   map[string]string{"action_id": action.ID, "error": err.Error()},
				CreatedAt: nowUTC(),
			})
			return events, err
		}

		events = append(events, Event{
			ID:        nextEventID(),
			Type:      "action_applied",
			Summary:   action.Description,
			Details:   map[string]string{"action_id": action.ID, "path": action.Path, "kind": action.Kind},
			CreatedAt: nowUTC(),
		})
	}

	events = append(events, Event{
		ID:        nextEventID(),
		Type:      "plan_verified",
		Summary:   "All planned actions verified",
		Details:   map[string]string{"plan_name": plan.Name},
		CreatedAt: nowUTC(),
	})

	return events, nil
}

func applyAction(action PlannedAction) error {
	switch action.Kind {
	case "mkdir":
		return os.MkdirAll(action.Path, 0o755)
	case "write":
		if err := os.MkdirAll(filepath.Dir(action.Path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(action.Path, []byte(action.Content), 0o644); err != nil {
			return err
		}
		_, err := os.Stat(action.Path)
		return err
	default:
		return fmt.Errorf("unsupported action kind %q", action.Kind)
	}
}

func isWithinWorkspace(workspacePath, targetPath string) bool {
	workspaceAbs, err := filepath.Abs(workspacePath)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(workspaceAbs, targetAbs)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." {
		return false
	}
	prefix := ".." + string(filepath.Separator)
	return !strings.HasPrefix(rel, prefix)
}

func scaffoldGoMod() string {
	return "module example.com/test-go-html\\n\\ngo 1.23\\n"
}

func scaffoldMainGo() string {
	return `package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("./web")))

	addr := ":8080"
	fmt.Printf("listening on http://localhost%s\\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
`
}

func scaffoldHTML() string {
	return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Go + HTML Test Project</title>
    <style>
      body {
        font-family: Georgia, "Times New Roman", serif;
        margin: 2rem;
        line-height: 1.4;
      }
      .card {
        max-width: 48rem;
        padding: 1.25rem;
        border: 1px solid #333;
      }
    </style>
  </head>
  <body>
    <div class="card">
      <h1>It works</h1>
      <p>This page is served by the Go HTTP server in <code>cmd/web/main.go</code>.</p>
    </div>
  </body>
</html>
`
}

func scaffoldREADME() string {
	return `# Test Go + HTML Project

## Run

` + "```bash" + `
go run ./cmd/web
` + "```" + `

Then open http://localhost:8080
`
}
