package worker

import (
	"context"
	"strconv"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestRepositoryVerificationAuthorityBindsExactStagePlanAndExpectedPost(t *testing.T) {
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
	firstStage := repositoryVerificationAuthorityStage(t, snapshot, analysis, contract, first.ID, 11)
	secondStage := repositoryVerificationAuthorityStage(t, snapshot, analysis, contract, first.ID, 12)
	t.Cleanup(func() { _ = firstStage.Cleanup() })
	t.Cleanup(func() { _ = secondStage.Cleanup() })
	firstAuthority, err := newRepositoryVerificationAuthority(
		snapshot.ID, contract.ID, commands, firstStage,
	)
	if err != nil {
		t.Fatal(err)
	}
	secondAuthority, err := newRepositoryVerificationAuthority(
		snapshot.ID, contract.ID, commands, secondStage,
	)
	if err != nil {
		t.Fatal(err)
	}
	if firstAuthority.stageID == secondAuthority.stageID ||
		firstAuthority.patchSHA256 == secondAuthority.patchSHA256 ||
		firstAuthority.expectedPostID == secondAuthority.expectedPostID {
		t.Fatalf("different correction stages share evidence authority: first=%+v second=%+v", firstAuthority, secondAuthority)
	}
	if firstAuthority.planID != secondAuthority.planID ||
		firstAuthority.contractID != contract.ID || firstAuthority.sourceSnapshotID != snapshot.ID {
		t.Fatalf("stable authority mismatch: first=%+v second=%+v", firstAuthority, secondAuthority)
	}
	prepared, err := newVerifiedRepositoryChangeStage(contract.ID, commands, secondStage)
	if err != nil {
		t.Fatal(err)
	}
	fromBundle, err := prepared.verificationAuthority(snapshot.ID, contract.ID, commands)
	if err != nil {
		t.Fatal(err)
	}
	if fromBundle != secondAuthority {
		t.Fatalf("bundle authority=%+v want=%+v", fromBundle, secondAuthority)
	}
}

func TestRepositoryExpectedPostRejectsMalformedContentIdentity(t *testing.T) {
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
	stage := repositoryVerificationAuthorityStage(t, snapshot, analysis, contract, first.ID, 11)
	t.Cleanup(func() { _ = stage.Cleanup() })
	expected := stage.ExpectedFiles()
	expected[0].SHA256 = "not-a-sha256"
	if _, err := repositoryExpectedPostID(
		snapshot.ID, stage.ChangedFileIDs(), expected,
	); err == nil {
		t.Fatal("malformed expected post content identity was accepted")
	}
}

func TestRepositoryAuthoritativeVerificationAuthorityRequiresFreshProjectionIdentity(t *testing.T) {
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
	stage := repositoryVerificationAuthorityStage(t, snapshot, analysis, contract, first.ID, 11)
	t.Cleanup(func() { _ = stage.Cleanup() })
	base, err := newRepositoryVerificationAuthority(snapshot.ID, contract.ID, commands, stage)
	if err != nil {
		t.Fatal(err)
	}
	authoritative, err := newRepositoryAuthoritativeVerificationAuthority(
		base, snapshot.ID, commands,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !base.allowsScope(repositoryVerificationStaged) ||
		base.allowsScope(repositoryVerificationAuthoritative) ||
		!authoritative.allowsScope(repositoryVerificationAuthoritative) ||
		authoritative.allowsScope(repositoryVerificationStaged) {
		t.Fatal("staged and authoritative verification authorities share a proof scope")
	}
	if got := authoritative.metadata()["repository_verification_snapshot_id"]; got != snapshot.ID {
		t.Fatalf("authoritative projection identity=%v want=%s", got, snapshot.ID)
	}
	if _, err := newRepositoryAuthoritativeVerificationAuthority(
		base, "snapshot_not-a-content-identity", commands,
	); err == nil {
		t.Fatal("authoritative verification accepted a malformed projection identity")
	}
}

func repositoryVerificationAuthorityStage(
	t *testing.T,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	contract repositoryfacts.ChangeContract,
	targetID string,
	value int,
) *changeapply.StagedChange {
	t.Helper()
	stage, err := changeapply.Plan(context.Background(), changeapply.Input{
		Snapshot: snapshot, Analysis: analysis, Contract: contract,
		Candidates: []changeapply.CandidateDeclaration{{
			SymbolID: targetID, Declaration: "func First() int { return " + strconv.Itoa(value) + " }",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return stage
}
