package codingobjective

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/omni"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestAmbientGoFlagsCannotBypassRepairAcceptance(t *testing.T) {
	t.Setenv("GOFLAGS", "-run=NoSuchTest")
	t.Setenv("GOWORK", filepath.Join(t.TempDir(), "attacker.work"))
	t.Setenv("GOENV", filepath.Join(t.TempDir(), "attacker.goenv"))
	fixture := numericCodingFixture(t)
	before := captureExactWorkspace(t, fixture.root)
	station := &recordingDeclarationStation{candidate: "func Fee() int { return baseFee() + 2 }"}
	applyCalls := 0
	result, err := runWithOperations(
		context.Background(), fixtureObjective(fixture), station,
		operations{apply: recordingApply(&applyCalls)},
	)
	if err == nil || !strings.Contains(err.Error(), "staged Go verification failed") {
		t.Fatalf("poisoned ambient environment bypassed failing test: result=%+v error=%v", result, err)
	}
	if result.ModelCalls != 1 || station.calls != 1 || applyCalls != 0 ||
		result.CommitOutcome != CommitNotAttempted || result.Complete {
		t.Fatalf("ambient bypass result=%+v station=%d apply=%d", result, station.calls, applyCalls)
	}
	assertExactWorkspaceUnchanged(t, fixture.root, before)
}

func TestInvalidAcceptanceFailsBeforeRepositoryOrStation(t *testing.T) {
	fixture := numericCodingFixture(t)
	before := captureExactWorkspace(t, fixture.root)
	for _, acceptance := range [][]AcceptancePredicate{
		nil, {}, {AcceptanceGoTestsPass, AcceptanceGoTestsPass}, {"model_judges_complete"},
	} {
		objective := fixtureObjective(fixture)
		objective.Acceptance = append([]AcceptancePredicate(nil), acceptance...)
		station := &recordingDeclarationStation{candidate: fixture.candidate}
		result, err := Run(context.Background(), objective, station)
		if !errors.Is(err, ErrInvalidObjective) || result.ModelCalls != 0 || station.calls != 0 ||
			result.CommitOutcome != CommitNotAttempted || result.Complete {
			t.Fatalf("acceptance=%v result=%+v station=%d error=%v", acceptance, result, station.calls, err)
		}
		assertExactWorkspaceUnchanged(t, fixture.root, before)
	}
}

func TestApplyErrorRetainsUnknownCommitAuthority(t *testing.T) {
	fixture := numericCodingFixture(t)
	before := snapshotFixture(t, fixture.root)
	station := &recordingDeclarationStation{candidate: fixture.candidate}
	result, err := runWithOperations(
		context.Background(), fixtureObjective(fixture), station,
		operations{apply: func(ctx context.Context, stage *changeapply.StagedChange) (omni.PatchApplyResult, error) {
			if _, applyErr := stage.ApplyVerified(ctx); applyErr != nil {
				return omni.PatchApplyResult{}, applyErr
			}
			return omni.PatchApplyResult{}, errors.New("commit receipt lost after apply")
		}},
	)
	if err == nil || !strings.Contains(err.Error(), "commit boundary may have been crossed") {
		t.Fatalf("ambiguous apply error=%v", err)
	}
	if result.Complete || result.CommitOutcome != CommitUnknown || result.StageID == "" ||
		result.PatchSHA256 == "" || len(result.ExpectedFiles) != 1 || !result.Satisfied {
		t.Fatalf("ambiguous apply result=%+v", result)
	}
	if reconcileErr := reconcileExpectedRepository(
		context.Background(), fixture.root, before, result.ExpectedFiles,
	); reconcileErr != nil {
		t.Fatalf("committed bytes do not match retained authority: %v", reconcileErr)
	}
}

func TestUnrelatedMutationAfterApplyPreventsCompletion(t *testing.T) {
	fixture := numericCodingFixture(t)
	station := &recordingDeclarationStation{candidate: fixture.candidate}
	result, err := runWithOperations(
		context.Background(), fixtureObjective(fixture), station,
		operations{apply: func(ctx context.Context, stage *changeapply.StagedChange) (omni.PatchApplyResult, error) {
			applied, applyErr := stage.ApplyVerified(ctx)
			if applyErr != nil {
				return omni.PatchApplyResult{}, applyErr
			}
			path := filepath.Join(fixture.root, "fee_test.go")
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return omni.PatchApplyResult{}, readErr
			}
			if writeErr := os.WriteFile(path, append(content, []byte("\n// unrelated drift\n")...), 0o600); writeErr != nil {
				return omni.PatchApplyResult{}, writeErr
			}
			return applied, nil
		}},
	)
	if err == nil || !strings.Contains(err.Error(), "exact expected indexed filesystem post-state") {
		t.Fatalf("unrelated postapply mutation error=%v", err)
	}
	if result.Complete || result.CommitOutcome != CommitUnknown || !result.Satisfied ||
		len(result.ExpectedFiles) != 1 {
		t.Fatalf("unrelated postapply mutation result=%+v", result)
	}
}
