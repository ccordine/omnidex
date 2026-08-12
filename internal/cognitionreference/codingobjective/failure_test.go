package codingobjective

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/omni"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestDeclarationFailuresDoNotMutateOrFallback(t *testing.T) {
	tests := []struct {
		name     string
		station  func(codingFixture) *recordingDeclarationStation
		contains string
	}{
		{
			name: "provider failure",
			station: func(codingFixture) *recordingDeclarationStation {
				return &recordingDeclarationStation{err: errors.New("fragment station unavailable")}
			},
			contains: "fragment station unavailable",
		},
		{
			name: "wrong job identity",
			station: func(fixture codingFixture) *recordingDeclarationStation {
				return &recordingDeclarationStation{candidate: fixture.candidate, resultID: strings.Repeat("0", 64)}
			},
			contains: "job id",
		},
		{
			name: "expanded declaration",
			station: func(fixture codingFixture) *recordingDeclarationStation {
				return &recordingDeclarationStation{candidate: fixture.candidate + "\n\nfunc Extra() {}"}
			},
			contains: "exactly one declaration",
		},
		{
			name: "unchanged declaration",
			station: func(codingFixture) *recordingDeclarationStation {
				return &recordingDeclarationStation{candidate: "func Fee() int { return baseFee() + 1 }"}
			},
			contains: "unchanged",
		},
		{
			name: "staged test failure",
			station: func(codingFixture) *recordingDeclarationStation {
				return &recordingDeclarationStation{candidate: "func Fee() int { return baseFee() + 2 }"}
			},
			contains: "staged Go verification",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := numericCodingFixture(t)
			before := captureExactWorkspace(t, fixture.root)
			station := test.station(fixture)
			applyCalls := 0
			result, err := runWithOperations(
				context.Background(), fixtureObjective(fixture), station,
				operations{apply: recordingApply(&applyCalls)},
			)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run() error=%v, want %q", err, test.contains)
			}
			if result.Complete || result.ModelCalls != 1 || station.calls != 1 {
				t.Fatalf("failed result=%+v station calls=%d", result, station.calls)
			}
			if result.CommitOutcome != CommitNotAttempted {
				t.Fatalf("precommit failure outcome=%q", result.CommitOutcome)
			}
			if applyCalls != 0 {
				t.Fatalf("rejected declaration invoked authoritative apply %d times", applyCalls)
			}
			assertExactWorkspaceUnchanged(t, fixture.root, before)
		})
	}
}

func TestDeterministicDiscoveryFailuresNeverCallStationOrMutate(t *testing.T) {
	t.Run("missing target", func(t *testing.T) {
		fixture := numericCodingFixture(t)
		before := captureExactWorkspace(t, fixture.root)
		objective := fixtureObjective(fixture)
		objective.Target = "example.com/numericfixture.Absent"
		station := &recordingDeclarationStation{candidate: fixture.candidate}
		result, err := Run(context.Background(), objective, station)
		if err == nil || !strings.Contains(err.Error(), "absent") {
			t.Fatalf("missing target error=%v", err)
		}
		assertNoDiscoveryMutation(t, fixture.root, before, result, station)
	})

	t.Run("bare target is not an exact identity", func(t *testing.T) {
		fixture := numericCodingFixture(t)
		before := captureExactWorkspace(t, fixture.root)
		objective := fixtureObjective(fixture)
		objective.Target = "Fee"
		station := &recordingDeclarationStation{candidate: fixture.candidate}
		result, err := Run(context.Background(), objective, station)
		if err == nil || !strings.Contains(err.Error(), "absent") {
			t.Fatalf("bare target error=%v", err)
		}
		assertNoDiscoveryMutation(t, fixture.root, before, result, station)
	})

	t.Run("unqualified duplicate target is absent", func(t *testing.T) {
		fixture := newCodingFixture(t, map[string]string{
			"go.mod":         "module example.com/ambiguous\n\ngo 1.24\n",
			"left/value.go":  "package left\n\nfunc Value() int { return 1 }\n",
			"right/value.go": "package right\n\nfunc Value() int { return 2 }\n",
		}, codingFixture{target: "Value", requirement: "change one exact value", directCapability: "helper"})
		before := captureExactWorkspace(t, fixture.root)
		station := &recordingDeclarationStation{candidate: "func Value() int { return 3 }"}
		result, err := Run(context.Background(), fixtureObjective(fixture), station)
		if err == nil || !strings.Contains(err.Error(), "absent") {
			t.Fatalf("unqualified duplicate target error=%v", err)
		}
		assertNoDiscoveryMutation(t, fixture.root, before, result, station)
	})

	t.Run("target has no direct test", func(t *testing.T) {
		fixture := numericCodingFixture(t)
		if err := os.Remove(filepath.Join(fixture.root, "fee_test.go")); err != nil {
			t.Fatal(err)
		}
		commitFixtureChange(t, fixture.root, "remove direct test")
		before := captureExactWorkspace(t, fixture.root)
		station := &recordingDeclarationStation{candidate: fixture.candidate}
		result, err := Run(context.Background(), fixtureObjective(fixture), station)
		if err == nil || !strings.Contains(err.Error(), "direct verification test") {
			t.Fatalf("missing direct test error=%v", err)
		}
		assertNoDiscoveryMutation(t, fixture.root, before, result, station)
	})

	t.Run("already satisfied repair", func(t *testing.T) {
		fixture := numericCodingFixture(t)
		failure := `package fee

import "testing"

func TestFee(t *testing.T) {
	if got := Fee(); got != 8 {
		t.Fatalf("baseline failure: %d", got)
	}
}
`
		if err := os.WriteFile(filepath.Join(fixture.root, "fee_test.go"), []byte(failure), 0o600); err != nil {
			t.Fatal(err)
		}
		commitFixtureChange(t, fixture.root, "make repair already satisfied")
		before := captureExactWorkspace(t, fixture.root)
		station := &recordingDeclarationStation{candidate: fixture.candidate}
		result, err := Run(context.Background(), fixtureObjective(fixture), station)
		if !errors.Is(err, ErrAlreadySatisfied) {
			t.Fatalf("already satisfied error=%v", err)
		}
		assertNoDiscoveryMutation(t, fixture.root, before, result, station)
	})
}

func TestNonCanonicalCompleteFileFailsBeforeApply(t *testing.T) {
	fixture := newCodingFixture(t, map[string]string{
		"go.mod":   "module example.com/unformatted\n\ngo 1.24\n",
		"value.go": "package value\n\nfunc helper()int{return 3}\n\nfunc Value() int { return helper() + 1 }\n",
		"value_test.go": `package value

import "testing"

func TestValue(t *testing.T) { if Value() != 3 { t.Fatal("wrong value") } }
`,
	}, codingFixture{
		target: "example.com/unformatted.Value", requirement: "Return the same value without the redundant addition.",
		candidate: "func Value() int { return helper() }", directCapability: "helper",
	})
	before := captureExactWorkspace(t, fixture.root)
	station := &recordingDeclarationStation{candidate: fixture.candidate}
	applyCalls := 0
	result, err := runWithOperations(
		context.Background(), fixtureObjective(fixture), station,
		operations{apply: recordingApply(&applyCalls)},
	)
	if err == nil || !strings.Contains(err.Error(), "not canonically formatted") {
		t.Fatalf("format failure error=%v", err)
	}
	if result.ModelCalls != 1 || station.calls != 1 || applyCalls != 0 || result.Complete {
		t.Fatalf("format failure result=%+v calls=%d apply=%d", result, station.calls, applyCalls)
	}
	assertExactWorkspaceUnchanged(t, fixture.root, before)
}

func TestCanceledObjectiveDoesNoWork(t *testing.T) {
	fixture := numericCodingFixture(t)
	before := captureExactWorkspace(t, fixture.root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	station := &recordingDeclarationStation{candidate: fixture.candidate}
	result, err := Run(ctx, fixtureObjective(fixture), station)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error=%v", err)
	}
	assertNoDiscoveryMutation(t, fixture.root, before, result, station)
}

func assertNoDiscoveryMutation(
	t *testing.T,
	root string,
	want exactWorkspaceState,
	result Result,
	station *recordingDeclarationStation,
) {
	t.Helper()
	if result.Complete || result.ModelCalls != 0 || station.calls != 0 {
		t.Fatalf("discovery failure result=%+v station calls=%d", result, station.calls)
	}
	if result.CommitOutcome != CommitNotAttempted {
		t.Fatalf("discovery failure commit outcome=%q", result.CommitOutcome)
	}
	assertExactWorkspaceUnchanged(t, root, want)
}

func commitFixtureChange(t *testing.T, root, message string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", message}} {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
}

func recordingApply(calls *int) func(context.Context, *changeapply.StagedChange) (omni.PatchApplyResult, error) {
	return func(context.Context, *changeapply.StagedChange) (omni.PatchApplyResult, error) {
		(*calls)++
		return omni.PatchApplyResult{}, errors.New("unexpected authoritative apply")
	}
}
