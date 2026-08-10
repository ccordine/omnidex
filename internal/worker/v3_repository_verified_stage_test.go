package worker

import (
	"context"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestVerifiedRepositoryChangeStageBindsContractAndOrderedProofPlan(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	contract, err := repositoryfacts.BuildChangeContract(
		snapshot, analysis, []repositoryfacts.ChangeRequest{{
			SymbolID: first.ID, RequirementQuote: "change First",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := changeapply.Plan(context.Background(), changeapply.Input{
		Snapshot: snapshot, Analysis: analysis, Contract: contract,
		Candidates: []changeapply.CandidateDeclaration{{
			SymbolID: first.ID, Declaration: "func First() int { return 2 }",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := newVerifiedRepositoryChangeStage(contract.ID, commands, stage)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Cleanup() })
	if err := prepared.RequireAuthority(contract.ID, commands); err != nil {
		t.Fatal(err)
	}
	if prepared.ID() != stage.ID() || prepared.PatchSHA256() != stage.PatchSHA256() ||
		len(prepared.ChangedFileIDs()) != 1 || len(prepared.ExpectedFiles()) != 1 {
		t.Fatalf("verified stage accessors do not preserve exact stage authority")
	}
	if err := prepared.RequireAuthority("different-contract", commands); err == nil ||
		!strings.Contains(err.Error(), "contract") {
		t.Fatalf("mismatched contract error=%v", err)
	}
	tampered := append([]testCommand(nil), commands...)
	tampered[0].Args = append([]string(nil), tampered[0].Args...)
	tampered[0].Args[4] = "^DifferentTest$"
	if err := prepared.RequireAuthority(contract.ID, tampered); err == nil ||
		!strings.Contains(err.Error(), "verification plan") {
		t.Fatalf("mismatched verification plan error=%v", err)
	}
	if err := prepared.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Cleanup(); err != nil {
		t.Fatalf("idempotent cleanup: %v", err)
	}
}
