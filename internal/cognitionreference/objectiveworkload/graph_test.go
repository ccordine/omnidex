package objectiveworkload_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference/objectiveworkload"
)

func TestRunRejectsEveryTamperedCodeOwnedGraphSurface(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*objectiveworkload.Workload)
	}{
		{
			name:   "workload identity",
			mutate: func(workload *objectiveworkload.Workload) { workload.ID = "invented" },
		},
		{
			name:   "authority digest",
			mutate: func(workload *objectiveworkload.Workload) { workload.Authority.SHA256 = "invented" },
		},
		{
			name:   "requirement quote",
			mutate: func(workload *objectiveworkload.Workload) { workload.Requirements[0].SourceQuote = "invented" },
		},
		{
			name:   "requirement digest",
			mutate: func(workload *objectiveworkload.Workload) { workload.Requirements[0].SHA256 = "invented" },
		},
		{
			name: "duplicate objective",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.Objectives[1].ID = workload.Objectives[0].ID
			},
		},
		{
			name: "dangling dependency",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.Objectives[0].DependsOn[0] = "missing"
			},
		},
		{
			name: "dependency cycle",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.Objectives[3].DependsOn = []objectiveworkload.ObjectiveID{"O000_root"}
			},
		},
		{
			name:   "parent",
			mutate: func(workload *objectiveworkload.Workload) { workload.Objectives[1].Parent = "O001_verify" },
		},
		{
			name:   "kind",
			mutate: func(workload *objectiveworkload.Workload) { workload.Objectives[1].Kind = "planner" },
		},
		{
			name: "acceptance",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.Objectives[1].Acceptance[0] = objectiveworkload.AcceptanceArtifactProduced
			},
		},
		{
			name: "status",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.Objectives[1].Status = objectiveworkload.ObjectiveComplete
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			workload := compileOne(t)
			test.mutate(&workload)
			operations := &scriptedOperations{}
			result, err := objectiveworkload.Run(
				context.Background(), workload, operations,
				objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 8},
			)
			if err == nil {
				t.Fatal("tampered workload unexpectedly ran")
			}
			if result.WorkloadID != "" || result.Complete || len(operations.calls) != 0 {
				t.Fatalf("result=%+v calls=%v", result, operations.calls)
			}
		})
	}
}

func TestRunRejectsCallerFabricatedRequirementLeafBeforeOperations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*objectiveworkload.Workload)
	}{
		{
			name: "untrimmed",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.Requirements[0].SourceQuote = " dashboard"
			},
		},
		{
			name: "oversized",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.Requirements[0].SourceQuote = strings.Repeat("x", 1025)
			},
		},
		{
			name: "mid rune start",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.Authority.Text = "é dashboard"
				workload.Requirements[0].Start = 1
				workload.Requirements[0].End = 10
			},
		},
		{
			name: "too many requirements",
			mutate: func(workload *objectiveworkload.Workload) {
				seed := workload.Requirements[0]
				workload.Requirements = make([]objectiveworkload.Requirement, 97)
				for index := range workload.Requirements {
					workload.Requirements[index] = seed
				}
			},
		},
		{
			name: "overlapping duplicate grounding",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.Authority.Text = "aaa"
				workload.Requirements[0].SourceQuote = "aa"
				workload.Requirements[0].Start = 0
				workload.Requirements[0].End = 2
			},
		},
		{
			name: "too many objectives",
			mutate: func(workload *objectiveworkload.Workload) {
				seed := workload.Objectives[0]
				workload.Objectives = make([]objectiveworkload.Objective, 290)
				for index := range workload.Objectives {
					workload.Objectives[index] = seed
				}
			},
		},
		{
			name: "oversized nested dependencies",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.Objectives[0].DependsOn = make([]objectiveworkload.ObjectiveID, 97)
			},
		},
		{
			name: "oversized parent identity",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.Objectives[1].Parent = objectiveworkload.ObjectiveID(strings.Repeat("x", 4096))
			},
		},
		{
			name: "oversized dependency identity",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.Objectives[0].DependsOn[0] = objectiveworkload.ObjectiveID(strings.Repeat("x", 4096))
			},
		},
		{
			name: "oversized root identity",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.RootObjectiveID = objectiveworkload.ObjectiveID(strings.Repeat("x", 4096))
			},
		},
		{
			name: "oversized workload identity",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.ID = objectiveworkload.WorkloadID(strings.Repeat("x", 4096))
			},
		},
		{
			name: "oversized requirement identity",
			mutate: func(workload *objectiveworkload.Workload) {
				workload.Requirements[0].ID = objectiveworkload.RequirementID(strings.Repeat("x", 4096))
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			workload := compileOne(t)
			test.mutate(&workload)
			operations := &scriptedOperations{}
			result, err := objectiveworkload.Run(
				context.Background(), workload, operations,
				objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 8},
			)
			if err == nil || result.WorkloadID != "" || result.Complete || len(operations.calls) != 0 {
				t.Fatalf("result=%+v calls=%v err=%v", result, operations.calls, err)
			}
		})
	}
}
