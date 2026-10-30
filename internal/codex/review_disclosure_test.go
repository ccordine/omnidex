package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Exercise the actual semantic parser: the same failed check can be a disclosed
// broader result, an item obligation, or an original-goal defect.
func TestInstalledReviewDisclosureScope(t *testing.T) {
	if os.Getenv("OMNI_TEST_PARSER_SIEVE") != "1" {
		t.Skip("OMNI_TEST_PARSER_SIEVE=1 is required")
	}
	provider, route := installedSieveProvider(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	task := &codingagent.Task{ID: "export", Goal: "Export invoices as CSV, preserving stored invoices, and verify round-trip behavior through the installed CLI. Run affected export regressions and disclose broader check failures accurately."}
	type fixture struct {
		name, response, kind string
		goal                 bool
		findings             bool
		call                 *codingagent.Assignment
	}
	cases := []fixture{
		{name: "broader disclosure", kind: "done", response: "Independent review verified the installed CSV export and round-trip behavior, preserved invoice records and passed export regressions. No unfinished export work found. Broader checks retain eight filesystem-fixture failures and a standards provenance mismatch; those results are disclosed in the evidence record."},
		{name: "no blockers with item verification limits", kind: "done", response: "Independent review verified installed CSV export and round-trip behavior. I found no blocking defect. Empty-source invocation remains unverified. Deliberately lost delivery remains untested; its HTTPS regression passes. These verification limits are documented."},
		{name: "explicit required verification gap", kind: "done", findings: true, response: "Review complete. Common CSV exports pass, but the required large-export installed check is unfinished and prevents fulfillment of this item. That required verification must be completed."},
		{name: "actual item defect", kind: "done", findings: true, response: "Review complete. CSV export fails to quote commas, corrupting the Acme, Inc. customer field; the required round-trip check fails. Broader filesystem checks also fail. The CSV corruption is unfinished work in this item."},
		{name: "required verification unfinished", kind: "error", response: "I inspected the implementation but could not run the required installed CLI round-trip verification because its executable is unavailable. The review remains incomplete."},
		{name: "original goal defect", kind: "done", goal: true, findings: true, response: "Final review complete. Invoice export works, but the original goal still requires passing storage and standards checks. Eight storage tests fail and the standards provenance mismatch remains unresolved. These required original-goal obligations are unfinished."},
	}
	if path := os.Getenv("OMNI_TEST_REVIEW_DISCLOSURE_CAPTURE"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var rows []struct {
			Entry codingagent.Entry `json:"payload_json"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range rows {
			if row.Entry.Phase == "call_opened" && row.Entry.Role == codingagent.ReviewResponse && row.Entry.Assignment != nil {
				call := *row.Entry.Assignment
				cases = append(cases, fixture{name: "captured production review", kind: "done", call: &call})
				found = true
			}
		}
		if !found {
			t.Fatal("capture has no original review parser assignment")
		}
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			call := codingagent.Assignment{Role: codingagent.ReviewResponse, Task: task, Response: fixture.response}
			if fixture.call != nil {
				call = *fixture.call
			}
			if fixture.goal {
				call.Task = nil
				call.Objective = &codingagent.Objective{Instruction: "Deliver invoice export with the required storage and standards checks passing."}
			}
			call.Root, call.Route, call.JobID, call.Generation = root, route, 65, 14
			result, err := provider.Run(t.Context(), call, func(codingagent.NativeEvent) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			var parsed codingagent.ReviewResponseResult
			if err := json.Unmarshal(result.Content, &parsed); err != nil {
				t.Fatal(err)
			}
			if string(parsed.Kind) != fixture.kind {
				t.Fatalf("kind=%s, want %s", parsed.Kind, fixture.kind)
			}
			if fixture.kind == "done" && (len(parsed.Findings) != 0) != fixture.findings {
				t.Fatalf("wrong scoped findings: %+v", parsed.Findings)
			}
			if parsed.Verification != call.Response {
				t.Fatal("parser discarded or rewrote the disclosed evidence")
			}
			t.Logf("kind=%s findings=%d", parsed.Kind, len(parsed.Findings))
		})
	}
}
