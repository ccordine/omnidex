package queue

import (
	"encoding/json"
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestCapturedLiveReviewRequiresItsImmutableParserRoute(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/planning-gauntlet/review-acceptance.json")
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Configuration codingagent.Configuration `json:"configuration"`
		Task          json.RawMessage           `json:"task"`
		Entries       []json.RawMessage         `json:"entries"`
	}
	if err := json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	task := normalizeHistoricalReviewTask(t, capture.Task)
	entries := []codingagent.Entry{}
	for _, rawEntry := range capture.Entries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(rawEntry, &fields); err != nil {
			t.Fatal(err)
		}
		if assignmentRaw := fields["assignment"]; len(assignmentRaw) > 0 && string(assignmentRaw) != "null" {
			var assignment map[string]json.RawMessage
			if err := json.Unmarshal(assignmentRaw, &assignment); err != nil {
				t.Fatal(err)
			}
			if taskRaw := assignment["task"]; len(taskRaw) > 0 && string(taskRaw) != "null" {
				normalized := normalizeHistoricalReviewTask(t, taskRaw)
				assignment["task"], err = json.Marshal(normalized)
				if err != nil {
					t.Fatal(err)
				}
			}
			fields["assignment"], err = json.Marshal(assignment)
			if err != nil {
				t.Fatal(err)
			}
		}
		normalized, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		var entry codingagent.Entry
		if err := json.Unmarshal(normalized, &entry); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	if err := validateCodingTaskAcceptanceJournal(entries, 1, capture.Configuration, task); err != nil {
		t.Fatalf("normalized historical execution/review/parser receipts were rejected: %v", err)
	}
	for _, route := range []codingagent.Route{{}, {Provider: "codex", Model: "other-parser", Effort: "low", ServiceTier: "default"}} {
		changed := capture.Configuration
		changed.LedgerParser = route
		if err := validateCodingTaskAcceptanceJournal(entries, 1, changed, task); err == nil {
			t.Fatal("missing or foreign parser authority accepted real review receipts")
		}
	}
}

// Translate only this old captured fixture. Preserve provider receipts/routes,
// acceptance text and guidance; no runtime compatibility path is introduced.
func normalizeHistoricalReviewTask(t *testing.T, raw json.RawMessage) codingagent.Task {
	t.Helper()
	var old struct {
		codingagent.Task
		Acceptance []string `json:"acceptance"`
		Guidance   []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"guidance"`
		Checks []struct {
			Argv []string `json:"argv"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	task := old.Task
	verification := []string{}
	for _, guidance := range old.Guidance {
		switch guidance.Kind {
		case "constraint":
			task.Constraints = append(task.Constraints, guidance.Text)
		case "verification_observation":
			verification = append(verification, guidance.Text)
		default:
			task.OwnerContext = append(task.OwnerContext, model.TaskContext{Text: guidance.Text, Provenance: model.LedgerProvenance{Kind: "captured_guidance", Reference: guidance.ID, Revision: 1}})
		}
	}
	for _, check := range old.Checks {
		if len(check.Argv) > 0 {
			verification = append(verification, strings.Join(check.Argv, " "))
		}
	}
	for i, text := range old.Acceptance {
		task.Acceptance = append(task.Acceptance, model.TaskAcceptance{ID: fmt.Sprint("captured-", i+1), Text: text, Verification: append([]string{}, verification...)})
	}
	return task
}
