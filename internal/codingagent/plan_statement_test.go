package codingagent

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPlanTaskStatementShowsExactMutationAndVerificationScope(t *testing.T) {
	task := Task{ID: "source", Goal: "Build the command", Acceptance: []string{"prints output"}, ReadFiles: []string{"go.mod"}, WriteFiles: []string{"main.go"}, DeleteFiles: []string{"old.go"}, Dependencies: []string{"module"}, Checks: []Check{{ID: "test", Argv: []string{"go", "test", "./..."}}}}
	statement, err := PlanTaskStatement(task)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Build the command", "prints output", "Read: go.mod", "Create/edit: main.go", "Delete: old.go", "After: module", "test=go test ./..."} {
		if !strings.Contains(statement, want) {
			t.Fatalf("authorization statement omitted %q: %s", want, statement)
		}
	}
}

func TestWorkloadRejectsTaskAuthorizationThatCannotFitTheLedger(t *testing.T) {
	workload := Workload{Tasks: []Task{{
		ID: "oversized", Goal: strings.Repeat("x", model.MaxCodingPlanStatementBytes), Acceptance: []string{"works"},
		WriteFiles: []string{"result.txt"}, RequirementIDs: []string{"objective"},
		Checks: []Check{{ID: "check", Argv: []string{"true"}}},
	}}, Checks: []Check{{ID: "overall", Argv: []string{"true"}}}}
	if err := workload.Validate(); err == nil || !strings.Contains(err.Error(), "authorization summary") {
		t.Fatalf("oversized user authorization projection error = %v", err)
	}
}
