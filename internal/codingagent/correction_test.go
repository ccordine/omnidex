package codingagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRejectedProposalsReturnToTheirOwnRoleWithConcreteNotes(t *testing.T) {
	controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Role == Execute {
			return ExecutionResult{Questions: []string{}}, nil
		}
		return cleanReview(), nil
	}, passingChecks)
	planner := controller.Providers["planner"]
	plans := 0
	controller.Providers["planner"] = providerFunc(func(ctx context.Context, call Assignment, writer FileWriter, event func(NativeEvent) error) (Result, error) {
		plans++
		result, err := planner.Run(ctx, call, writer, event)
		if plans == 1 {
			invalid := fixtureWorkload()
			invalid.Tasks[0].ReadFiles = []string{"undeclared.txt"}
			result.Content, _ = json.Marshal(invalid)
		} else if len(call.ValidationNotes) != 1 || !strings.Contains(call.ValidationNotes[0], "ungranted dependency") {
			t.Fatal("planner did not receive mechanical rejection")
		}
		return result, err
	})
	reviewer := controller.Providers["reviewer"]
	reviews := 0
	controller.Providers["reviewer"] = providerFunc(func(ctx context.Context, call Assignment, writer FileWriter, event func(NativeEvent) error) (Result, error) {
		reviews++
		result, err := reviewer.Run(ctx, call, writer, event)
		if reviews == 1 {
			result.Content, _ = json.Marshal(ReviewResult{Findings: []Finding{{TaskID: call.Task.ID, Requirement: "invented scope", Observation: "additional feature wanted"}}, Questions: []string{}})
		} else if reviews == 2 && (len(call.ValidationNotes) != 1 || !strings.Contains(call.ValidationNotes[0], "new acceptance")) {
			t.Fatal("reviewer did not receive narrow rejection")
		}
		return result, err
	})
	if err := controller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if plans != 2 {
		t.Fatalf("planner did not converge: %d calls", plans)
	}
	phases := map[string]int{}
	for _, entry := range journal.entries {
		phases[entry.Phase]++
	}
	if phases["plan_rejected"] != 1 || phases["review_rejected"] != 1 || phases["plan_frozen"] != 1 || phases["overall_accepted"] != 1 {
		t.Fatalf("invalid proposal changed accepted authority: %v", phases)
	}
}

func TestFileWriterRequiresExactTaskGrantAndActiveContext(t *testing.T) {
	writer := taskFileWriter{environment: fixtureEnvironment{}, files: []string{"allowed.txt"}}
	for _, path := range []string{"other.txt", "../allowed.txt", ".git/config"} {
		if err := writer.WriteFile(context.Background(), FileWrite{Path: path, Content: "effect"}); err == nil {
			t.Fatalf("ungranted write %s accepted", path)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := writer.WriteFile(ctx, FileWrite{Path: "allowed.txt", Content: "effect"}); err == nil {
		t.Fatal("canceled writer accepted an effect")
	}
}

func TestParentOutcomeDoesNotExpandEachIsolatedTask(t *testing.T) {
	workload := fixtureWorkload()
	workload.Tasks[1].RequirementIDs = []string{"r1"}
	controller, _ := fixtureController(t, workload, func(call Assignment) (any, error) {
		if call.Task != nil && (len(call.Task.Acceptance) != 1 || len(call.Requirements) != 0 || call.Objective != nil || call.Workload != nil) {
			t.Fatal("isolated task received unrelated parent context")
		}
		if call.Role == Execute {
			return ExecutionResult{Questions: []string{}}, nil
		}
		return cleanReview(), nil
	}, passingChecks)
	controller.Requirements = []Requirement{{ID: "r1", Outcome: "A and B work together"}}
	if err := controller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}
