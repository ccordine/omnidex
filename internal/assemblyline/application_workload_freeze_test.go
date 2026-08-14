package assemblyline

import (
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestFrozenApplicationWorkloadHasDeterministicCodeOwnedIdentity(t *testing.T) {
	t.Parallel()
	input := applicationWorkloadTestInput()
	draft := applicationWorkloadTestDraft()
	first, err := FreezeApplicationWorkload(input, draft)
	if err != nil {
		t.Fatal(err)
	}
	second, err := FreezeApplicationWorkload(input, cloneApplicationWorkloadDraft(draft))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("freeze is not deterministic:\n%+v\n%+v", first, second)
	}
	decoded, err := hex.DecodeString(first.SHA256)
	if err != nil || len(decoded) != 32 || first.SHA256 != strings.ToLower(first.SHA256) {
		t.Fatalf("workload hash=%q error=%v", first.SHA256, err)
	}
	wantIDs := []string{"task_001", "task_002", "task_003"}
	gotIDs := make([]string, 0, len(first.Tasks))
	for _, task := range first.Tasks {
		gotIDs = append(gotIDs, task.ID)
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("code-owned task IDs=%v want %v", gotIDs, wantIDs)
	}
	if !reflect.DeepEqual(first.Tasks[2].DependsOn, []string{"task_001", "task_002"}) {
		t.Fatalf("code did not resolve requirement dependencies to task identities: %+v", first.Tasks[2])
	}
	if err := ValidateFrozenApplicationWorkload(input, first); err != nil {
		t.Fatal(err)
	}
}

func TestFrozenApplicationWorkloadRejectsMutationAndAuthorityDrift(t *testing.T) {
	t.Parallel()
	input := applicationWorkloadTestInput()
	frozen, err := FreezeApplicationWorkload(input, applicationWorkloadTestDraft())
	if err != nil {
		t.Fatal(err)
	}

	mutated := frozen
	mutated.Tasks = append([]FrozenApplicationTask(nil), frozen.Tasks...)
	mutated.Tasks[1].Objective = "Mutated after freeze."
	if err := ValidateFrozenApplicationWorkload(input, mutated); err == nil {
		t.Fatal("accepted task mutation under the original workload hash")
	}

	drifted := input
	drifted.Requirements = append([]Requirement(nil), input.Requirements...)
	drifted.Requirements[1].SourceQuote = "different accepted authority"
	if err := ValidateFrozenApplicationWorkload(drifted, frozen); err == nil {
		t.Fatal("accepted changed requirement authority under a frozen workload")
	}
}

func TestFreezeApplicationWorkloadValidatesCodeOwnedCoverageAndGraph(t *testing.T) {
	t.Parallel()
	input := applicationWorkloadTestInput()
	valid := applicationWorkloadTestDraft()
	tests := map[string]func(*ApplicationWorkloadDraft){
		"missing requirement": func(value *ApplicationWorkloadDraft) { value.Tasks = value.Tasks[:2] },
		"duplicate requirement": func(value *ApplicationWorkloadDraft) {
			value.Tasks[2].RequirementID = "requirement_002"
		},
		"unknown requirement": func(value *ApplicationWorkloadDraft) {
			value.Tasks[2].RequirementID = "requirement_999"
		},
		"not source ordered": func(value *ApplicationWorkloadDraft) {
			value.Tasks[0], value.Tasks[1] = value.Tasks[1], value.Tasks[0]
		},
		"unknown dependency": func(value *ApplicationWorkloadDraft) {
			value.Tasks[2].DependsOn = []string{"requirement_999"}
		},
		"self dependency": func(value *ApplicationWorkloadDraft) {
			value.Tasks[2].DependsOn = []string{"requirement_003"}
		},
		"dependency order": func(value *ApplicationWorkloadDraft) {
			value.Tasks[2].DependsOn = []string{"requirement_002", "requirement_001"}
		},
		"duplicate dependency": func(value *ApplicationWorkloadDraft) {
			value.Tasks[2].DependsOn = []string{"requirement_001", "requirement_001"}
		},
		"cycle": func(value *ApplicationWorkloadDraft) {
			value.Tasks[0].DependsOn = []string{"requirement_002"}
			value.Tasks[1].DependsOn = []string{"requirement_001"}
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			candidate := cloneApplicationWorkloadDraft(valid)
			mutate(&candidate)
			if _, err := FreezeApplicationWorkload(input, candidate); err == nil {
				t.Fatalf("accepted invalid code-owned workload %+v", candidate)
			}
		})
	}
}

func TestFrozenApplicationWorkloadBuildsDeterministicDependencyWaves(t *testing.T) {
	t.Parallel()
	input := applicationWorkloadTestInput()
	draft := applicationWorkloadTestDraft()
	draft.Tasks[2].DependsOn = nil
	frozen, err := FreezeApplicationWorkload(input, draft)
	if err != nil {
		t.Fatal(err)
	}
	waves, err := BuildApplicationWorkloadWaves(input, frozen)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"task_001", "task_003"}, {"task_002"}}
	if !reflect.DeepEqual(waves, want) {
		t.Fatalf("waves=%v want %v", waves, want)
	}
}

func TestApplicationTaskContextContainsAuthoritativeBaselineCurrentTaskAndDirectDependencies(t *testing.T) {
	t.Parallel()
	input := applicationWorkloadTestInput()
	draft := applicationWorkloadTestDraft()
	draft.Tasks[2].DependsOn = []string{"requirement_002"}
	frozen, err := FreezeApplicationWorkload(input, draft)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ProjectApplicationTaskContext(input, frozen, "task_003")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"workload_sha256": frozen.SHA256,
		"surface":         "browser_application",
		"product_quote":   "browser operations console",
		"task": map[string]any{
			"task_id":             "task_003",
			"requirement_id":      "requirement_003",
			"requirement_quote":   "export printable summaries",
			"objective":           "Implement printable export.",
			"required_behaviors":  []any{"Create a printable summary from visible records."},
			"acceptance_criteria": []any{"A user can open a printable summary."},
		},
		"dependencies": []any{
			map[string]any{
				"task_id":           "task_002",
				"requirement_id":    "requirement_002",
				"requirement_quote": "filter records quickly",
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projection=%s", raw)
	}
	for _, forbidden := range []string{
		"SECRET_FULL_CONVERSATION", "file_path", "tool_catalog",
		"group records by status", "Implement status grouping.",
		"Implement record filtering.", "Visible records match the selected filter.",
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("task projection contains unrelated authority %q: %s", forbidden, raw)
		}
	}
	if _, err := ProjectApplicationTaskContext(input, frozen, "task_999"); err == nil {
		t.Fatal("projected context for an unknown task")
	}
}
