package codex

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestInstalledExecutionOwnerInputRouting(t *testing.T) {
	if os.Getenv("OMNI_TEST_PARSER_SIEVE") != "1" {
		t.Skip("OMNI_TEST_PARSER_SIEVE=1 is required")
	}
	provider, route := installedSieveProvider(t)
	task := codingagent.Task{ID: "invoice-import", Goal: "Implement invoice CSV import, verify it, and install the verified build through the normal release path.",
		Constraints: []string{"Preserve unrelated edits in the authoritative checkout.", "Do not create alternate source checkouts or commit unrelated edits without owner permission."}}
	question := "The importer and its checks are complete. The task remains incomplete: the verified build is not installed. The qualified release guard rejects the dirty authoritative tree. Which approved source/delivery path should I use while preserving the unrelated edits?"
	for _, test := range []struct {
		name, response, want string
		clarifications       []codingagent.TaskClarification
	}{
		{name: "required-delivery-decision", response: question, want: "question"},
		{name: "required-authorization", response: "Implementation and checks are complete. Normal delivery requires a separate clean build checkout, which the task constraints forbid without permission. May I create one disposable build checkout while preserving the authoritative tree and unrelated edits?", want: "permission"},
		{name: "already-answered-decision", response: question, want: "continue", clarifications: []codingagent.TaskClarification{{QuestionCallID: 31, AnswerEvidenceID: 32, Question: question, Answer: "Use the prepared single disposable build checkout at /workspace/release-build. I authorize that checkout and its pinned local build commit. Preserve the authoritative tree and unrelated edits."}}},
		{name: "unfinished-authorized-verification", response: "Implementation and automated checks pass. Required browser verification is not run yet. The browser is available and no owner decision or missing permission prevents running it. Should I continue with those required checks?", want: "continue"},
		{name: "completed-item-awaiting-review", response: "The importer is installed and all required verification passed. The separate independent review has not run yet.", want: "done"},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := codingagent.Assignment{Role: codingagent.ExecutionResponse, JobID: 17, Generation: 1,
				Root: t.TempDir(), Route: route, Task: &task, Response: test.response, Clarifications: test.clarifications}
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
			if parsed.Kind != test.want {
				t.Fatalf("required owner input was lost or invented: got %s, want %s: %s", parsed.Kind, test.want, result.Content)
			}
		})
	}
}
