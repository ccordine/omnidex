package objectiveworkload_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/cognitionreference/objectiveworkload"
)

func TestOrdinaryTextCompilesAndRunsAsRecursiveCodeOwnedWorkload(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name      string
		authority string
		quotes    []string
	}{
		{
			name:      "pantry",
			authority: "  Keep a pantry inventory and print a restock summary.  ",
			quotes:    []string{"pantry inventory", "restock summary"},
		},
		{
			name:      "timer",
			authority: "\nShow lap history with keyboard controls.\t",
			quotes:    []string{"lap history", "keyboard controls"},
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			station := &scriptedPartitionStation{steps: newPartitionScript(fixture.authority, fixture.quotes...)}
			compiled, err := objectiveworkload.Compile(
				context.Background(), fixture.authority, station,
				objectiveworkload.CompileLimits{MaxStationCalls: 16},
			)
			if err != nil {
				t.Fatal(err)
			}
			if !compiled.Compiled || compiled.EvidenceClass != objectiveworkload.EvidencePrimitiveContaminatedNonAutonomy {
				t.Fatalf("compile result=%+v", compiled)
			}
			if compiled.Authority.Text != fixture.authority {
				t.Fatalf("authority changed: %q", compiled.Authority.Text)
			}
			if compiled.StationCalls != 2+len(fixture.quotes) || len(compiled.Gaps) != compiled.StationCalls {
				t.Fatalf("station calls=%d gaps=%d", compiled.StationCalls, len(compiled.Gaps))
			}
			for _, gap := range compiled.Gaps {
				if gap.Status != objectiveworkload.GapResolved || gap.OutputSHA256 == "" ||
					gap.CompilationID != compiled.CompilationID || gap.FinalWorkloadID != compiled.Workload.ID ||
					!gap.ResponseObserved || !gap.ResponseWithinBounds || !gap.ResponseJobIDMatches {
					t.Fatalf("resolved gap lacks exact output authority: %+v", gap)
				}
			}
			if len(compiled.Workload.Requirements) != len(fixture.quotes) ||
				len(compiled.Workload.Objectives) != 1+3*len(fixture.quotes) {
				t.Fatalf("compiled workload=%+v", compiled.Workload)
			}
			for index, requirement := range compiled.Workload.Requirements {
				if requirement.SourceQuote != fixture.quotes[index] ||
					fixture.authority[requirement.Start:requirement.End] != requirement.SourceQuote {
					t.Fatalf("requirement %d=%+v", index, requirement)
				}
			}
			assertPartitionOnlySurface(t, fixture.authority, station.prompts)

			operations := &scriptedOperations{}
			run, err := objectiveworkload.Run(
				context.Background(), compiled.Workload, operations,
				objectiveworkload.RunLimits{MaxTransitions: 32, MaxDepth: 8},
			)
			if err != nil {
				t.Fatal(err)
			}
			if !run.Complete || run.ModelCalls != 0 || run.StationCalls != 0 {
				t.Fatalf("run=%+v", run)
			}
			if run.EvidenceClass != objectiveworkload.EvidencePrimitiveContaminatedNonAutonomy {
				t.Fatalf("evidence class=%q", run.EvidenceClass)
			}
			wantOperations := []string{
				"materialize:R001", "verify:R001",
				"materialize:R002", "verify:R002",
			}
			if !reflect.DeepEqual(operations.calls, wantOperations) {
				t.Fatalf("operation calls=%v want %v", operations.calls, wantOperations)
			}
			wantTransitions := []objectiveworkload.ObjectiveID{
				"O001_materialize", "O001_verify", "O001_requirement",
				"O002_materialize", "O002_verify", "O002_requirement",
				"O000_root",
			}
			gotTransitions := make([]objectiveworkload.ObjectiveID, len(run.Trace))
			for index, transition := range run.Trace {
				gotTransitions[index] = transition.ObjectiveID
			}
			if !reflect.DeepEqual(gotTransitions, wantTransitions) {
				t.Fatalf("transitions=%v want %v", gotTransitions, wantTransitions)
			}
			if len(run.Artifacts) != len(fixture.quotes) || run.DeterministicOperationCalls != 2*len(fixture.quotes) {
				t.Fatalf("artifacts=%d operations=%d", len(run.Artifacts), run.DeterministicOperationCalls)
			}
		})
	}
}

func assertPartitionOnlySurface(t *testing.T, authority string, prompts []string) {
	t.Helper()
	if len(prompts) == 0 {
		t.Fatal("no partition prompts recorded")
	}
	for _, prompt := range prompts {
		withoutAuthority := strings.ReplaceAll(prompt, authority, "")
		for _, forbidden := range []string{
			"tool catalog", "tool call", "action arguments", "file path", "ledger", "working set",
			"objective graph", "depends_on", "acceptance", "completion status",
		} {
			if strings.Contains(strings.ToLower(withoutAuthority), forbidden) {
				t.Fatalf("partition prompt exposes %q:\n%s", forbidden, prompt)
			}
		}
	}
	job, err := assemblyline.NewRequirementPartitionJob(assemblyline.RequirementPartitionInput{
		SourceText: authority, Mode: assemblyline.RequirementExtractFeatures,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, schema, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"action", "argument", "tool", "memory", "ledger", "objective", "completion"} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Fatalf("partition schema exposes %q: %s", forbidden, raw)
		}
	}
}
