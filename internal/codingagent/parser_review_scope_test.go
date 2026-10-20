package codingagent

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestFrozenReviewScopeAndOriginalEvidence(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/parser-sieve/job14-review.json")
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Task     Task   `json:"task"`
		Response string `json:"response"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, response, kind, quote string }{
		{"job14", saved.Response, "done", ""},
		{"acceptance follows review", "Review complete. All required checks pass. Code-owned acceptance remains pending.", "done", ""},
		{"required failure", "Required protocol verification failed with exit 1. Review could not finish.", "error", ""},
		{"finding", "Review complete. agentPermissions leaves review read-only at turn startup.", "done", "agentPermissions leaves review read-only at turn startup."},
		{"qualification finding", "Review complete. fixture_test.go accepts an invalid result; qualification failed with exit 1. Acceptance remains unresolved.", "done", "fixture_test.go accepts an invalid result; qualification failed with exit 1."},
		{"missing evidence", "I inspected the source.", "error", ""},
		{"unperformed verification", "Review is unfinished. I inspected the source but have not performed required verification.", "error", ""},
		{"inaccessible verification", "The required verifier is inaccessible; review remains unfinished.", "error", ""},
		{"question", "Which required fixture endpoint should I use? I need that answer to finish required verification.", "question", ""},
		{"permission", "May I access the required fixture?", "permission", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := Assignment{Role: ReviewResponse, Task: &saved.Task, Response: tc.response, Route: Route{Provider: "fixture", Model: "fixture"}}
			provider := sieveTestProvider(t, func(leaf Assignment) any {
				step := leaf.ParserStep
				if step.TaskInstructions != saved.Task.Goal {
					t.Fatal("lost frozen task instructions")
				}
				prompt, err := RenderParserPrompt(leaf)
				if err != nil {
					t.Fatal(err)
				}
				background, err := RenderParserContext(leaf)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(background, "task_instructions") {
					t.Fatal("missing task context")
				}
				if step.Stage == "scan" {
					if !strings.Contains(prompt, "unrequested live-model evidence") || !strings.Contains(prompt, "Evaluate the independent REVIEW assignment only") || !strings.Contains(prompt, "not a prerequisite for review completion") {
						t.Fatal("missing task-scoped classification")
					}
					kind := tc.kind
					if tc.name == "job14" && step.BlockID != "block_1" {
						kind = "ignore"
					}
					return scanOracle(step.Source, kind)
				}
				if !strings.Contains(prompt, "exact frozen task_instructions") {
					t.Fatal("missing finding scope")
				}
				findings := parserReviewFindings{Findings: []struct {
					Quote string `json:"quote"`
					File  string `json:"file"`
				}{}}
				if tc.quote != "" {
					findings.Findings = append(findings.Findings, struct {
						Quote string `json:"quote"`
						File  string `json:"file"`
					}{Quote: tc.quote})
				}
				return findings
			})
			result, err := RunParserSieve(t.Context(), call, func(NativeEvent) error { return nil }, provider)
			if err != nil {
				t.Fatal(err)
			}
			var parsed ReviewResponseResult
			err = decodeContract(result.Content, &parsed)
			if err != nil {
				t.Fatal(err)
			}
			if string(parsed.Kind) != tc.kind || parsed.Verification != tc.response {
				t.Fatalf("lost outcome or source: %+v", parsed)
			}
			want := 0
			if tc.quote != "" {
				want = 1
			}
			if len(parsed.Findings) != want {
				t.Fatalf("findings=%+v", parsed.Findings)
			}
			if want == 1 && (parsed.Findings[0].TaskID != saved.Task.ID || parsed.Findings[0].Observation != tc.quote) {
				t.Fatal("finding changed")
			}
		})
	}
}

func TestReviewUnknownMeaningIsNotIgnored(t *testing.T) {
	_, err := parserScanParts(ReviewResponse, "Ambiguous required check.", scanOracle("Ambiguous required check.", "unknown"))
	if err == nil {
		t.Fatal("unresolved review meaning accepted")
	}
}

func TestReviewFindingsDoNotOverrideUnfinishedReview(t *testing.T) {
	for _, kind := range []string{"error", "question", "permission"} {
		source := "Review found a defect.\n\nRequired review work remains outstanding."
		call := Assignment{Role: ReviewResponse, Task: &Task{ID: "task", Goal: "Implement and verify the requested result."}, Response: source, Route: Route{Provider: "fixture", Model: "fixture"}}
		result, err := RunParserSieve(t.Context(), call, func(NativeEvent) error { return nil }, sieveTestProvider(t, func(leaf Assignment) any {
			if leaf.ParserStep.Stage == "scan" {
				state := "done"
				if leaf.ParserStep.BlockID == "block_2" {
					state = kind
				}
				return scanOracle(leaf.ParserStep.Source, state)
			}
			findings := []map[string]string{}
			if leaf.ParserStep.BlockID == "block_1" {
				findings = append(findings, map[string]string{"quote": "Review found a defect.", "file": ""})
			}
			return map[string]any{"findings": findings}
		}))
		if err != nil {
			t.Fatal(err)
		}
		var parsed ReviewResponseResult
		if err := json.Unmarshal(result.Content, &parsed); err != nil {
			t.Fatal(err)
		}
		if string(parsed.Kind) != kind || len(parsed.Findings) != 1 || parsed.Verification != source {
			t.Fatalf("unfinished review hidden: %+v", parsed)
		}
	}
}

func TestReviewSieveRejectsInvalidFindingsAndScanOutput(t *testing.T) {
	for _, bad := range []any{
		map[string]any{"findings": nil},
		map[string]any{"findings": []map[string]string{{"quote": "invented defect", "file": ""}}},
		map[string]any{"findings": []map[string]string{{"quote": "Review complete.", "file": "invented.go"}}},
		map[string]any{"units": map[string]any{}},
	} {
		call := Assignment{Role: ReviewResponse, Task: &Task{ID: "task", Goal: "Verify the requested result."}, Response: "Review complete.", Route: Route{Provider: "fixture", Model: "fixture"}}
		_, err := RunParserSieve(t.Context(), call, func(NativeEvent) error { return nil }, sieveTestProvider(t, func(leaf Assignment) any {
			if leaf.ParserStep.Stage == "scan" {
				if _, ok := bad.(map[string]any)["units"]; ok {
					return bad
				}
				return scanOracle(leaf.ParserStep.Source, "done")
			}
			return bad
		}))
		if err == nil {
			t.Fatalf("invalid review output accepted: %+v", bad)
		}
	}
}
