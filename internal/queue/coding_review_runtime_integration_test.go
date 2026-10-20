package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Scripted semantic fixture: real queue, PostgreSQL evidence, Docker and browser;
// this does not measure a live model's implementation or review judgment.
type reviewRuntimeFixture struct {
	t        *testing.T
	roles    []codingagent.Role
	expected string
}

func (p *reviewRuntimeFixture) Attest(context.Context) error { return nil }
func (p *reviewRuntimeFixture) Run(ctx context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	content := json.RawMessage(`"Verified task-local bytes and isolated browser runtime."`)
	switch call.Role {
	case codingagent.Execute, codingagent.Review:
		if call.Session != nil || call.Task == nil || call.Task.ID != "task" {
			return codingagent.Result{}, fmt.Errorf("not a fresh task-local assignment")
		}
		p.roles = append(p.roles, call.Role)
		file := filepath.Join(call.Root, "fixture.txt")
		if call.Role == codingagent.Execute {
			if err := os.WriteFile(file, []byte(p.expected), 0600); err != nil {
				return codingagent.Result{}, err
			}
		}
		before, err := os.ReadFile(file)
		if err != nil || string(before) != p.expected {
			return codingagent.Result{}, fmt.Errorf("unexpected task bytes: %q %v", before, err)
		}
		// No mounts, network, persistent profile or external server. JavaScript must
		// execute in an actual browser before its DOM can contain the expected value.
		name := fmt.Sprintf("omnidex-review-grant-browser-%s-%d", call.Role, time.Now().UnixNano())
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = exec.CommandContext(cleanup, "docker", "rm", "-f", name).Run()
		}()
		args := []string{"run", "--rm", "--name", name, "--network", "none", "--memory", "512m", "--memory-swap", "512m", "--cpus", "1", "--pids-limit", "128", "--read-only", "--tmpfs", "/tmp:rw,size=128m", "--shm-size", "128m", "-e", "HOME=/tmp", "-e", "XDG_CONFIG_HOME=/tmp/config", "-e", "XDG_CACHE_HOME=/tmp/cache", "--entrypoint", "/ms-playwright/chromium_headless_shell-1228/chrome-headless-shell-linux64/chrome-headless-shell", "mcr.microsoft.com/playwright:v1.61.0-noble", "--disable-background-networking", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage", "--no-first-run", "--user-data-dir=/tmp/profile", "--dump-dom", "data:text/html,<body><script>document.body.textContent=6*7</script>"}
		output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if err != nil || !strings.Contains(string(output), "<body>42</body>") {
			return codingagent.Result{}, fmt.Errorf("isolated browser: %v: %s", err, output)
		}
		after, err := os.ReadFile(file)
		if err != nil || string(after) != string(before) {
			return codingagent.Result{}, fmt.Errorf("verification modified implementation")
		}
		p.t.Logf("%s: docker %s => exit 0, DOM <body>42</body>; implementation unchanged", call.Role, strings.Join(args, " "))
		raw, _ := json.Marshal(map[string]string{"role": string(call.Role), "command": "docker " + strings.Join(args, " "), "observed": "exit 0; DOM <body>42</body>; implementation unchanged"})
		if err := event(codingagent.NativeEvent{Frame: raw}); err != nil {
			return codingagent.Result{}, err
		}
	case codingagent.ExecutionResponse:
		content = json.RawMessage(`{"kind":"done"}`)
	case codingagent.ReviewResponse:
		content = json.RawMessage(`{"kind":"done","findings":[],"verification":"Read exact task bytes and ran isolated Chromium JavaScript DOM check; implementation unchanged."}`)
	default:
		return codingagent.Result{}, fmt.Errorf("unexpected role %s", call.Role)
	}
	if err := event(codingagent.NativeEvent{Frame: json.RawMessage(`{"fixture":"scripted-role-response"}`)}); err != nil {
		return codingagent.Result{}, err
	}
	return codingagent.Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, ThreadID: "fresh-" + string(call.Role), TurnID: "turn-" + string(call.Role), Content: content}, nil
}

func TestFrozenTaskExecutionIndependentReviewRuntime(t *testing.T) {
	if os.Getenv("OMNI_TEST_REVIEW_RUNTIME") != "1" {
		t.Skip("OMNI_TEST_REVIEW_RUNTIME=1 is required")
	}
	for _, expected := range []string{"alpha\nbeta\n", "{\"enabled\":true}\n"} {
		t.Run(strings.TrimSpace(expected), func(t *testing.T) {
			_, repository := freshUserIntentRepository(t)
			command, fixture := userIntentFixture(t, repository, fmt.Sprintf("Write fixture.txt containing exactly %q and verify an isolated browser runtime.", expected))
			request, _ := saveIntent(t, repository, command, fixture.created.Revision, "Approve and start this exact fixture.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			if _, err := repository.ExecuteUserIntent(ctx, request.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "review-runtime-fixture")
			if err != nil || claim == nil || claim.Step.Action != "v3_coding" {
				t.Fatalf("claim=%+v error=%v", claim, err)
			}
			provider := &reviewRuntimeFixture{t: t, expected: expected}
			controller := codingagent.Controller{JobID: claim.Job.ID, Generation: claim.Authority.Generation, Root: command.WorkspaceRoot, Configuration: repository.codingAgents, Environment: provider, Evidence: repository.CodingAgentEvidence(claim.Authority), Providers: map[string]codingagent.Provider{"codex": provider}}
			task, err := repository.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
			if err != nil || task == nil {
				t.Fatalf("task=%+v error=%v", task, err)
			}
			if err := controller.RunTask(ctx, *task); err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(provider.roles) != "[execute review]" {
				t.Fatalf("roles=%v", provider.roles)
			}
			if err := repository.AcceptCodingLedgerTask(ctx, claim.Authority, task.ID); err != nil {
				t.Fatal(err)
			}
			remaining, err := repository.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
			if err != nil || remaining != nil {
				t.Fatalf("remaining=%+v error=%v", remaining, err)
			}
		})
	}
}
