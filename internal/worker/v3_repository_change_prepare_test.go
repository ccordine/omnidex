package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestPrepareVerifiedRepositoryChangeCorrectsOnlyOwnerAndRestagesAllCandidates(t *testing.T) {
	t.Parallel()
	snapshot, analysis, contract, candidates := repositoryCorrectionFixture(t)
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	second := existingRepositoryVerificationSymbol(t, analysis, "Second")
	var stagedRoots []string
	verifications := 0
	corrections := 0
	prepared, err := prepareVerifiedRepositoryChangeWithOperations(
		context.Background(), snapshot, analysis, contract, candidates, commands,
		repositoryChangePrepareOperations{
			plan: repositoryCorrectionPlanner(snapshot, analysis, contract, &stagedRoots),
			verify: func(stage *changeapply.StagedChange, _ repositoryGoCorrectionOwnership) error {
				verifications++
				firstContent, readErr := os.ReadFile(filepath.Join(stage.Workspace(), "first.go"))
				if readErr != nil {
					return readErr
				}
				secondContent, readErr := os.ReadFile(filepath.Join(stage.Workspace(), "sub", "second.go"))
				if readErr != nil {
					return readErr
				}
				if !strings.Contains(string(secondContent), "return 12") {
					t.Fatalf("accepted non-owner candidate was not preserved:\n%s", secondContent)
				}
				if verifications == 1 {
					if !strings.Contains(string(firstContent), "return 11") {
						t.Fatalf("initial owner candidate missing:\n%s", firstContent)
					}
					return &repositoryGoVerificationFailure{
						targetSymbolID: first.ID, diagnostic: "got 11, want 1",
					}
				}
				if !strings.Contains(string(firstContent), "return 13") {
					t.Fatalf("corrected owner candidate was not restaged:\n%s", firstContent)
				}
				return nil
			},
			correct: func(target repositoryfacts.ChangeTarget, current, diagnostic string) (string, error) {
				corrections++
				if target.SymbolID != first.ID || current != candidates[first.ID] || diagnostic != "got 11, want 1" {
					t.Fatalf("correction target=%q current=%q diagnostic=%q", target.SymbolID, current, diagnostic)
				}
				return "func First() int { return 13 }", nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.RequireAuthority(contract.ID, commands); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Cleanup() })
	if verifications != 2 || corrections != 1 || len(stagedRoots) != 2 {
		t.Fatalf("verifications=%d corrections=%d stages=%d", verifications, corrections, len(stagedRoots))
	}
	if _, err := os.Stat(stagedRoots[0]); !os.IsNotExist(err) {
		t.Fatalf("failed stage was not cleaned: %v", err)
	}
	if _, err := os.Stat(stagedRoots[1]); err != nil {
		t.Fatalf("final verified stage was not kept live: %v", err)
	}
	if candidates[first.ID] != "func First() int { return 11 }" ||
		candidates[second.ID] != "func Second() int { return 12 }" {
		t.Fatalf("caller-owned candidates were mutated: %#v", candidates)
	}
}

func TestPrepareVerifiedRepositoryChangeEnforcesCorrectionCapAndNoProgress(t *testing.T) {
	t.Parallel()
	snapshot, analysis, contract, candidates := repositoryCorrectionFixture(t)
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	t.Run("hard cap", func(t *testing.T) {
		var stagedRoots []string
		corrections := 0
		_, err := prepareVerifiedRepositoryChangeWithOperations(
			context.Background(), snapshot, analysis, contract, candidates, commands,
			repositoryChangePrepareOperations{
				plan: repositoryCorrectionPlanner(snapshot, analysis, contract, &stagedRoots),
				verify: func(*changeapply.StagedChange, repositoryGoCorrectionOwnership) error {
					return &repositoryGoVerificationFailure{targetSymbolID: first.ID, diagnostic: "still wrong"}
				},
				correct: func(_ repositoryfacts.ChangeTarget, _, _ string) (string, error) {
					corrections++
					return "func First() int { return " + string(rune('1'+corrections)) + " }", nil
				},
			},
		)
		if err == nil || !strings.Contains(err.Error(), "correction round limit") {
			t.Fatalf("correction cap error=%v", err)
		}
		if corrections != maxRepositoryGoVerificationCorrectionRounds ||
			len(stagedRoots) != maxRepositoryGoVerificationCorrectionRounds+1 {
			t.Fatalf("corrections=%d stages=%d", corrections, len(stagedRoots))
		}
		assertRepositoryCorrectionStagesCleaned(t, stagedRoots)
	})
	t.Run("no progress", func(t *testing.T) {
		var stagedRoots []string
		_, err := prepareVerifiedRepositoryChangeWithOperations(
			context.Background(), snapshot, analysis, contract, candidates, commands,
			repositoryChangePrepareOperations{
				plan: repositoryCorrectionPlanner(snapshot, analysis, contract, &stagedRoots),
				verify: func(*changeapply.StagedChange, repositoryGoCorrectionOwnership) error {
					return &repositoryGoVerificationFailure{targetSymbolID: first.ID, diagnostic: "still wrong"}
				},
				correct: func(_ repositoryfacts.ChangeTarget, current, _ string) (string, error) {
					return current, nil
				},
			},
		)
		if err == nil || !strings.Contains(err.Error(), "no progress") {
			t.Fatalf("no-progress error=%v", err)
		}
		assertRepositoryCorrectionStagesCleaned(t, stagedRoots)
	})
}

func TestPrepareVerifiedRepositoryChangeDoesNotCorrectInfrastructureFailure(t *testing.T) {
	t.Parallel()
	snapshot, analysis, contract, candidates := repositoryCorrectionFixture(t)
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	var stagedRoots []string
	corrections := 0
	_, err = prepareVerifiedRepositoryChangeWithOperations(
		context.Background(), snapshot, analysis, contract, candidates, commands,
		repositoryChangePrepareOperations{
			plan: repositoryCorrectionPlanner(snapshot, analysis, contract, &stagedRoots),
			verify: func(*changeapply.StagedChange, repositoryGoCorrectionOwnership) error {
				return errors.New("sandbox integrity evidence failure")
			},
			correct: func(repositoryfacts.ChangeTarget, string, string) (string, error) {
				corrections++
				return "", nil
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "sandbox integrity evidence failure") || corrections != 0 {
		t.Fatalf("fatal verification error=%v corrections=%d", err, corrections)
	}
	assertRepositoryCorrectionStagesCleaned(t, stagedRoots)
}

func repositoryCorrectionPlanner(
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	contract repositoryfacts.ChangeContract,
	stagedRoots *[]string,
) func(context.Context, []changeapply.CandidateDeclaration) (*changeapply.StagedChange, error) {
	return func(ctx context.Context, declarations []changeapply.CandidateDeclaration) (*changeapply.StagedChange, error) {
		stage, err := changeapply.Plan(ctx, changeapply.Input{
			Snapshot: snapshot, Analysis: analysis, Contract: contract, Candidates: declarations,
		})
		if err == nil {
			*stagedRoots = append(*stagedRoots, stage.Workspace())
		}
		return stage, err
	}
}

func assertRepositoryCorrectionStagesCleaned(t *testing.T, roots []string) {
	t.Helper()
	for _, root := range roots {
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatalf("failed correction stage %q remains: %v", root, err)
		}
	}
}
