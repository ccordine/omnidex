package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/manifest"
)

// These are transport qualification fixtures, not workload autonomy evidence.
func TestInstalledManifestPlanningRoles(t *testing.T) {
	if os.Getenv("OMNI_TEST_MANIFEST") != "1" {
		t.Skip("OMNI_TEST_MANIFEST=1 is required")
	}
	client, _ := installedAgentClient(t)
	configuration := manifest.Defaults().Coding()
	for _, request := range []string{
		"Build a small local command-line program that normalizes supplied whitespace. Include its verification in the proposed work.",
		"Add a reusable integer range-intersection helper to a library and verify its behavior.",
	} {
		t.Run(request, func(t *testing.T) {
			evidence := &roundTripEvidence{}
			controller := codingagent.Controller{JobID: 91, Generation: 1, Root: t.TempDir(), Configuration: configuration,
				Providers: map[string]codingagent.Provider{"codex": client}, Evidence: evidence, Environment: roundTripEnvironment{}}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
			defer cancel()
			proposal, err := controller.ProposePlanningLedger(ctx, codingagent.PlanningTurn{
				OperationID: "manifest-planning-fixture", OwnerTurnID: "manifest-owner-fixture", Text: request,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(proposal.Ledger.Tasks) == 0 {
				t.Fatal("planning returned no implementation tasks")
			}
			for _, task := range proposal.Ledger.Tasks {
				t.Logf("proposed task: %s", task.Goal)
			}
			seenPlanner, seenParser := false, false
			for _, entry := range evidence.entries {
				if entry.Assignment == nil {
					continue
				}
				call := entry.Assignment
				switch call.Role {
				case codingagent.Plan:
					seenPlanner = call.Route == configuration.Planner
				case codingagent.PlanningInitial:
					seenParser = call.Route == configuration.LedgerParser
				}
				if entry.Result != nil && !json.Valid(entry.Result.Content) {
					t.Fatal("provider returned invalid content")
				}
			}
			if !seenPlanner || !seenParser {
				t.Fatal("manifest role routes were not used")
			}
		})
	}
}

func TestInstalledManifestExecutionReviewRoles(t *testing.T) {
	if os.Getenv("OMNI_TEST_MANIFEST") != "1" {
		t.Skip("OMNI_TEST_MANIFEST=1 is required")
	}
	client, _ := installedAgentClient(t)
	configuration := manifest.Defaults().Coding()
	for _, fixture := range []struct{ name, file, goal, expected string }{
		{"text", "result.txt", "Create result.txt containing exactly the two lines alpha and beta, each followed by a newline. Verify its bytes with a shell command. This is the entire task.", "alpha\nbeta\n"},
		{"json", "settings.json", "Create settings.json containing exactly {\"enabled\":true,\"count\":3} followed by one newline. Verify its bytes with a shell command. This is the entire task.", "{\"enabled\":true,\"count\":3}\n"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			evidence := &roundTripEvidence{}
			controller := codingagent.Controller{JobID: 92, Generation: 1, Root: root, Configuration: configuration,
				Providers: map[string]codingagent.Provider{"codex": client}, Evidence: evidence, Environment: roundTripEnvironment{}}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
			defer cancel()
			task := codingagent.Task{ID: "manifest-role-fixture", Goal: fixture.goal, WriteFiles: []string{fixture.file}}
			if err := controller.RunTask(ctx, task); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(root, fixture.file))
			if err != nil || string(raw) != fixture.expected {
				t.Fatalf("execution did not produce the task's exact bytes: %q %v", raw, err)
			}
			seenExecution, seenReview, seenAcceptance := false, false, false
			for _, entry := range evidence.entries {
				if entry.Assignment != nil {
					call := entry.Assignment
					switch call.Role {
					case codingagent.Execute:
						seenExecution = call.Route == configuration.Executor && call.Session == nil
					case codingagent.Review:
						seenReview = call.Route == configuration.Reviewer && call.Session == nil
					case codingagent.ExecutionResponse, codingagent.ReviewResponse:
						if call.Route != configuration.LedgerParser {
							t.Fatal("response parser did not use the manifest route")
						}
					}
				}
				if entry.Phase == "task_accepted" && entry.TaskID == task.ID {
					seenAcceptance = true
				}
			}
			if !seenExecution || !seenReview || !seenAcceptance {
				t.Fatalf("missing execution, independent review or persisted acceptance: %v %v %v", seenExecution, seenReview, seenAcceptance)
			}
			t.Log("Full-permission low execution and independent xhigh review completed without owner input.")
		})
	}
}
