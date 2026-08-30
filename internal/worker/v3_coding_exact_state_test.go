package worker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestDirectCodingExactStateAuthorityBindsPlannerProvenStateAndCommands(t *testing.T) {
	root := t.TempDir()
	content := []byte("package main\n\nfunc main() {}\n")
	if err := os.WriteFile(filepath.Join(root, "main.go"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	source, err := workspacefacts.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	entry := source.Entries[0]
	desired := []workspacefacts.DesiredFileState{{
		Path: "main.go",
		Source: &workspacefacts.ExactSourceFile{
			EntryID: entry.ID, SHA256: entry.SHA256, Size: entry.Size, Mode: entry.Mode,
		},
		Present: true, Content: content, Mode: entry.Mode,
	}}
	ownerID := "coding_" + strings.Repeat("a", 64)
	if _, err := workspacefacts.PlanMutation(t.Context(), source, ownerID, desired); !errors.Is(err, workspacefacts.ErrDesiredStateAlreadyExact) {
		t.Fatalf("planner exact-state proof error=%v", err)
	}
	commands := cloneWorkspaceRoleCommands(
		goCommandLineVerificationCommandSet(), workspaceVerificationPrimary,
	)
	authority, err := newDirectCodingExactStateAuthority(source, ownerID, commands)
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.validate(source, commands); err != nil {
		t.Fatal(err)
	}
	tampered := append([]testCommand(nil), commands...)
	tampered[0].Args = []string{"test", "./other"}
	if err := authority.validate(source, tampered); err == nil {
		t.Fatal("exact-state authority accepted a different verification command")
	}
}

func TestDirectCodingVerifiedNoDeltaProofIsDistinctFromMutationProof(t *testing.T) {
	commands := cloneWorkspaceRoleCommands(
		goCommandLineVerificationCommandSet(), workspaceVerificationPrimary,
	)
	authority := directCodingExactStateAuthority{
		ID:           "workspace_exact_" + strings.Repeat("a", 64),
		Verification: mustWorkspaceVerificationPlan(t, commands),
	}
	receiptSHA256, err := directCodingExactStateReceiptSHA256(
		authority, []int64{11, 12, 13}, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	replayedReceiptSHA256, err := directCodingExactStateReceiptSHA256(
		authority, []int64{21, 22, 23}, true,
	)
	if err != nil || replayedReceiptSHA256 != receiptSHA256 {
		t.Fatalf("replayed receipt=%q initial=%q err=%v", replayedReceiptSHA256, receiptSHA256, err)
	}
	noDelta := directCodingVerification{
		Passed: true, TestsPassed: true,
		Commands: directCodingCommandLabels(commands), EvidenceIDs: []int64{11, 12, 13},
		ExactStateAuthorityID: authority.ID, ExactStateReceiptSHA256: receiptSHA256,
	}
	if err := noDelta.validate(); err != nil {
		t.Fatal(err)
	}
	noDeltaProof := directCodingVerificationProof(41, noDelta)
	if !validDirectCodingVerificationProof(41, noDeltaProof) ||
		!strings.Contains(noDeltaProof.URI, "/workspace-exact/workspace_exact_") {
		t.Fatalf("verified no-delta proof=%+v", noDeltaProof)
	}
	replayed := noDelta
	replayed.EvidenceIDs = []int64{21, 22, 23}
	if replayedProof := directCodingVerificationProof(41, replayed); replayedProof != noDeltaProof {
		t.Fatalf("verified no-delta replay proof=%+v initial=%+v", replayedProof, noDeltaProof)
	}
	mutation := noDelta
	mutation.ExactStateAuthorityID = ""
	mutation.ExactStateReceiptSHA256 = ""
	mutation.MutationOperationID = "workspace_mutation_" + strings.Repeat("b", 64)
	mutation.MutationReceiptSHA256 = strings.Repeat("c", 64)
	mutationProof := directCodingVerificationProof(41, mutation)
	if !validDirectCodingVerificationProof(41, mutationProof) ||
		mutationProof.URI == noDeltaProof.URI || mutationProof.Hash == noDeltaProof.Hash {
		t.Fatalf("mutation proof=%+v no_delta=%+v", mutationProof, noDeltaProof)
	}
}

func mustWorkspaceVerificationPlan(
	t *testing.T,
	commands []testCommand,
) queue.WorkspaceMutationVerificationPlan {
	t.Helper()
	plan, err := workspaceVerificationPlanForCommands(commands)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
