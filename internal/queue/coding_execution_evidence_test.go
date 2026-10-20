package queue

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Validate actual retained production journals, including the new exact report
// attachment, without asking a model to reclassify a prior answer.
func TestCapturedExecutionProofEvidenceAcceptance(t *testing.T) {
	dir := os.Getenv("OMNI_EXECUTION_PROOF_EVIDENCE")
	if dir == "" {
		t.Skip("OMNI_EXECUTION_PROOF_EVIDENCE required")
	}
	for _, name := range []string{"automatic", "waiver", "enabling"} {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join(dir, name+".jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			decoder := json.NewDecoder(f)
			var entries []codingagent.Entry
			for decoder.More() {
				var row struct {
					Entry *codingagent.Entry `json:"persisted_entry"`
				}
				if err := decoder.Decode(&row); err != nil {
					t.Fatal(err)
				}
				if row.Entry != nil {
					entries = append(entries, *row.Entry)
				}
			}
			var task codingagent.Task
			var route codingagent.Route
			var review *codingagent.Assignment
			var unresolved []codingagent.Entry
			for i, e := range entries {
				if e.Assignment != nil {
					task = *e.Assignment.Task
					route = e.Assignment.Route
					if e.Role == codingagent.Review {
						review = e.Assignment
					}
				}
				if e.Phase == "task_reviewed" && unresolved == nil {
					unresolved = entries[:i+1]
				}
			}
			if review == nil || review.ExecutionReport == nil || len(unresolved) == 0 {
				t.Fatal("capture lacks correction or review evidence")
			}
			cfg := codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route}
			if err := validateCodingTaskAcceptanceJournal(entries, 1, cfg, task); err != nil {
				t.Fatal("actual final journal rejected", err)
			}
			if err := validateCodingTaskAcceptanceJournal(unresolved, 1, cfg, task); err == nil {
				t.Fatal("unresolved proof accepted")
			}
			original := *review.ExecutionReport
			for _, mutate := range []func(){func() { review.ExecutionReport.Response = "rewritten report" }, func() { review.ExecutionReport.CallID++ }, func() { review.ExecutionReport.Round++ }} {
				mutate()
				if err := validateCodingTaskAcceptanceJournal(entries, 1, cfg, task); err == nil {
					t.Fatal("changed original execution evidence accepted")
				}
				*review.ExecutionReport = original
			}
		})
	}
}
