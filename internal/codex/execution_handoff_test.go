package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestInstalledExecutionReviewHandoff(t *testing.T) {
	if os.Getenv("OMNI_TEST_PARSER_SIEVE") != "1" {
		t.Skip("OMNI_TEST_PARSER_SIEVE=1 is required")
	}
	provider, route := installedSieveProvider(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	task := &codingagent.Task{ID: "export", Goal: "Implement CSV export and verify it through the installed CLI. Report unavailable live paths accurately without counting fixtures as installed passes."}
	type handoffCase struct {
		name, want string
		call       codingagent.Assignment
	}
	cases := []handoffCase{
		{name: "completed handoff with disclosed limits", want: "done", call: codingagent.Assignment{JobID: 17, Generation: 1, Task: task, Response: "CSV export is implemented, installed, and verified through the real CLI. The implementation and evidence are ready for independent review. Additional platform variants and deliberately dropped responses were not exercised; those remain disclosed verification limits."}},
		{name: "completed summary without handoff phrase", want: "done", call: codingagent.Assignment{JobID: 17, Generation: 1, Task: task, Response: "Fixed CSV export and delivered it to the existing installation. Verified export through the real CLI; regression tests pass. Additional graphical variants remain untested."}},
		{name: "acceptance disclosure without next action", want: "done", call: codingagent.Assignment{JobID: 17, Generation: 1, Task: task, Response: "CSV export is installed. Added the combined recovery regression; focused and race checks pass. Acceptance remains incomplete: extra graphical variants remain unverified. Existing sessions are preserved."}},
		{name: "required execution work remains", want: "continue", call: codingagent.Assignment{JobID: 17, Generation: 1, Task: task, Response: "CSV export still corrupts quoted fields. I need to fix that implementation and rerun the required installed CLI check. All tools and authority are available."}},

		{name: "unfinished implementation without next-action phrasing", want: "continue", call: codingagent.Assignment{JobID: 17, Generation: 1, Task: task, Response: "The assignment remains incomplete. Inspection confirms that export still corrupts quoted fields, contrary to the approved CSV format. I made no implementation changes. Installed verification remains unperformed."}},
		{name: "ordinary failed attempt with authorized work remaining", want: "continue", call: codingagent.Assignment{JobID: 17, Generation: 1, Task: task, Response: "Implemented the export correction, but the assignment is still incomplete. Installed verification did not run: direct CLI bootstrap lacked the normal startup configuration, and copying the file into the running container failed because its filesystem is read-only. The normal build and deployment tools remain available. Required installed delivery and round-trip verification remain unfinished."}},
	}
	if path := os.Getenv("OMNI_TEST_EXECUTION_HANDOFF_CAPTURE"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var captured struct {
			Call codingagent.Assignment `json:"call"`
		}
		if err := json.Unmarshal(raw, &captured); err != nil {
			t.Fatal(err)
		}
		if captured.Call.Role != codingagent.ExecutionResponse || captured.Call.Task == nil || captured.Call.Response == "" {
			t.Fatal("capture is not an execution parser assignment")
		}
		cases = append(cases, handoffCase{name: "captured completed handoff", want: "done", call: captured.Call})
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			call := fixture.call
			call.Role, call.Root, call.Route = codingagent.ExecutionResponse, root, route
			result, err := provider.Run(t.Context(), call, func(codingagent.NativeEvent) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			var parsed struct {
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(result.Content, &parsed); err != nil {
				t.Fatal(err)
			}
			if parsed.Kind != fixture.want {
				t.Fatalf("kind=%s, want %s", parsed.Kind, fixture.want)
			}
			t.Logf("kind=%s", parsed.Kind)
		})
	}
}
