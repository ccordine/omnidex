package codex

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestReadOnlyCheckScopePreservesOneTaskPacket(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review} {
		t.Run(string(role), func(t *testing.T) {
			scope := agentTestScope(t)
			scope.Role = role
			projected, err := readOnlyCheckScope(scope)
			if err != nil {
				t.Fatal(err)
			}
			expected := scope
			expected.WriteFiles = nil
			expected.DeleteFiles = nil
			if role == codingagent.Execute {
				expected.Role = codingagent.Review
			}
			if !reflect.DeepEqual(projected, expected) {
				t.Fatal("check projection changed the task packet")
			}
		})
	}
}

func TestReadOnlyCheckScopeRejectsAnythingButOneTaskPacket(t *testing.T) {
	for _, mutate := range []func(*codingagent.Assignment){
		func(scope *codingagent.Assignment) { scope.Role = codingagent.PlanningInitial },
		func(scope *codingagent.Assignment) { scope.Role = "unregistered" },
		func(scope *codingagent.Assignment) { scope.Task = nil },
		func(scope *codingagent.Assignment) {
			scope.Objective = &codingagent.Objective{Instruction: "full objective"}
		},
		func(scope *codingagent.Assignment) { scope.ReadFiles = append(scope.ReadFiles, "unassigned.go") },
	} {
		scope := agentTestScope(t)
		scope.Role = codingagent.Review
		mutate(&scope)
		if _, err := readOnlyCheckScope(scope); err == nil {
			t.Fatal("check scope accepted context outside one task packet")
		}
	}
}
