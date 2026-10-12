package codex

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestReadOnlyCheckScopePreservesTaskAndOverallAuthority(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review, codingagent.OverallReview} {
		t.Run(string(role), func(t *testing.T) {
			scope := agentTestScope(t)
			scope.Role = role
			if role == codingagent.OverallReview {
				scope.Workload = &codingagent.Workload{Tasks: []codingagent.Task{*scope.Task}}
				scope.Task = nil
				scope.Requirements = []codingagent.Requirement{{ID: "parent", Outcome: "The approved application works."}}
				scope.Objective = &codingagent.Objective{Instruction: "The original accepted objective."}
				scope.ArtifactBindings = []codingagent.ArtifactBinding{{Token: "ARTIFACT_1", Value: "source.go"}}
			}
			projected, err := readOnlyCheckScope(scope)
			if err != nil {
				t.Fatalf("valid %s check scope was rejected: %v", role, err)
			}
			expected := scope
			expected.WriteFiles = nil
			if role == codingagent.Execute {
				expected.Role = codingagent.Review
			}
			if !reflect.DeepEqual(projected, expected) {
				t.Fatal("read-only checks changed the assigned task or overall context")
			}
			if !reflect.DeepEqual(scope.WriteFiles, []string{"source.go"}) {
				t.Fatal("check projection mutated the caller's source authority")
			}
		})
	}
}

func TestReadOnlyCheckScopeRejectsUnassignedOrNonnarrowContext(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*codingagent.Assignment)
	}{
		{"planner", func(scope *codingagent.Assignment) { scope.Role = codingagent.Plan }},
		{"unknown role", func(scope *codingagent.Assignment) { scope.Role = "unregistered" }},
		{"missing task", func(scope *codingagent.Assignment) { scope.Task = nil }},
		{"parent objective in task", func(scope *codingagent.Assignment) {
			scope.Objective = &codingagent.Objective{Instruction: "Unassigned parent objective"}
		}},
		{"parent workload in task", func(scope *codingagent.Assignment) {
			scope.Workload = &codingagent.Workload{Tasks: []codingagent.Task{*scope.Task}}
		}},
		{"unrelated read file", func(scope *codingagent.Assignment) {
			scope.ReadFiles = []string{"contract.txt", "unassigned.go"}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			scope := agentTestScope(t)
			scope.Role = codingagent.Review
			test.mutate(&scope)
			if _, err := readOnlyCheckScope(scope); err == nil {
				t.Fatal("deterministic checks accepted an unauthorized scope")
			}
		})
	}
}
