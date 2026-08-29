package assemblyline

import (
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestFrozenApplicationWorkloadIsDeterministicExactRequirementProjection(t *testing.T) {
	t.Parallel()
	specification := applicationWorkloadTestSpecification()
	first, err := FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	second, err := FreezeApplicationWorkload(specification)
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
	want := []FrozenApplicationTask{
		{ID: "task_001", RequirementID: "requirement_001", RequirementQuote: "group records by status"},
		{ID: "task_002", RequirementID: "requirement_002", RequirementQuote: "filter records quickly"},
		{ID: "task_003", RequirementID: "requirement_003", RequirementQuote: "export printable summaries"},
	}
	if !reflect.DeepEqual(first.Tasks, want) {
		t.Fatalf("tasks=%+v want %+v", first.Tasks, want)
	}
	if err := ValidateFrozenApplicationWorkload(first); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFrozenApplicationWorkloadFor(specification, first); err != nil {
		t.Fatal(err)
	}
}

func TestFrozenApplicationWorkloadRejectsMutationAndAuthorityDrift(t *testing.T) {
	t.Parallel()
	specification := applicationWorkloadTestSpecification()
	frozen, err := FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}

	mutated := frozen
	mutated.Tasks = append([]FrozenApplicationTask(nil), frozen.Tasks...)
	mutated.Tasks[1].RequirementQuote = "mutated after freeze"
	if err := ValidateFrozenApplicationWorkload(mutated); err == nil {
		t.Fatal("accepted task mutation under the original workload hash")
	}

	drifted := specification
	drifted.Requirements = append([]Requirement(nil), specification.Requirements...)
	drifted.Requirements[1].SourceQuote = "different accepted authority"
	if err := ValidateFrozenApplicationWorkloadFor(drifted, frozen); err == nil {
		t.Fatal("accepted changed requirement authority under a frozen workload")
	}
}

func TestFrozenApplicationWorkloadContainsNoSemanticExpansionFields(t *testing.T) {
	t.Parallel()
	frozen, err := FreezeApplicationWorkload(applicationWorkloadTestSpecification())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"objective", "required_behaviors", "acceptance_criteria", "depends_on", "dependencies",
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("frozen workload contains obsolete planner field %q: %s", forbidden, raw)
		}
	}
}

func TestApplicationTaskContextContainsOnlyExactRequirementContract(t *testing.T) {
	t.Parallel()
	frozen, err := FreezeApplicationWorkload(applicationWorkloadTestSpecification())
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ProjectApplicationTaskContext(frozen, "task_003")
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
			"task_id":           "task_003",
			"requirement_id":    "requirement_003",
			"requirement_quote": "export printable summaries",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projection=%s", raw)
	}
	for _, forbidden := range []string{
		"objective", "behavior", "acceptance", "dependency", "group records by status",
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("task projection contains unrelated or expanded authority %q: %s", forbidden, raw)
		}
	}
	if _, err := ProjectApplicationTaskContext(frozen, "task_999"); err == nil {
		t.Fatal("projected context for an unknown task")
	}
}
