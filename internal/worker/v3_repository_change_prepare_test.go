package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestPrepareVerifiedRepositoryChangeKeepsOneSuccessfulStage(t *testing.T) {
	t.Parallel()
	snapshot, analysis, contract, candidates := repositoryPreparationFixture(t)
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	second := existingRepositoryVerificationSymbol(t, analysis, "Second")
	var stagedRoots []string
	verifications := 0
	prepared, err := prepareVerifiedRepositoryChangeWithOperations(
		context.Background(), snapshot, analysis, contract, candidates, commands,
		repositoryChangePrepareOperations{
			plan: repositoryPreparationPlanner(snapshot, analysis, contract, &stagedRoots),
			verify: func(stage *changeapply.StagedChange) error {
				verifications++
				firstContent, readErr := os.ReadFile(filepath.Join(stage.DeltaRoot(), "first.go"))
				if readErr != nil {
					return readErr
				}
				secondContent, readErr := os.ReadFile(filepath.Join(stage.DeltaRoot(), "sub", "second.go"))
				if readErr != nil {
					return readErr
				}
				if !strings.Contains(string(firstContent), "return 11") ||
					!strings.Contains(string(secondContent), "return 12") {
					t.Fatalf("stage omitted exact candidates:\n%s\n%s", firstContent, secondContent)
				}
				return nil
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
	if verifications != 1 || len(stagedRoots) != 1 {
		t.Fatalf("verifications=%d stages=%d", verifications, len(stagedRoots))
	}
	if _, err := os.Stat(stagedRoots[0]); err != nil {
		t.Fatalf("verified stage was not kept live: %v", err)
	}
	if candidates[first.ID] != "func First() int { return 11 }" ||
		candidates[second.ID] != "func Second() int { return 12 }" {
		t.Fatalf("caller-owned candidates were mutated: %#v", candidates)
	}
}

func TestPrepareVerifiedRepositoryChangeFailsOnceAndCleansStage(t *testing.T) {
	t.Parallel()
	snapshot, analysis, contract, candidates := repositoryPreparationFixture(t)
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	for name, failure := range map[string]error{
		"verification failure":   errors.New("got 11, want 1"),
		"infrastructure failure": errors.New("sandbox integrity evidence failure"),
	} {
		t.Run(name, func(t *testing.T) {
			var stagedRoots []string
			verifications := 0
			_, err := prepareVerifiedRepositoryChangeWithOperations(
				context.Background(), snapshot, analysis, contract, candidates, commands,
				repositoryChangePrepareOperations{
					plan: repositoryPreparationPlanner(snapshot, analysis, contract, &stagedRoots),
					verify: func(*changeapply.StagedChange) error {
						verifications++
						return failure
					},
				},
			)
			if err == nil || !strings.Contains(err.Error(), failure.Error()) {
				t.Fatalf("staged failure=%v", err)
			}
			if verifications != 1 || len(stagedRoots) != 1 {
				t.Fatalf("verifications=%d stages=%d", verifications, len(stagedRoots))
			}
			assertRepositoryPreparationStagesCleaned(t, stagedRoots)
		})
	}
}

func repositoryPreparationFixture(
	t *testing.T,
) (repositoryfacts.Snapshot, repositoryfacts.Analysis, repositoryfacts.ChangeContract, map[string]string) {
	t.Helper()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	requests := make([]repositoryfacts.ChangeRequest, 0, 2)
	candidates := make(map[string]string)
	for index, name := range []string{"First", "Second"} {
		symbol := existingRepositoryVerificationSymbol(t, analysis, name)
		requests = append(requests, repositoryfacts.ChangeRequest{
			SymbolID: symbol.ID, RequirementQuote: "change " + name,
		})
		candidates[symbol.ID] = "func " + name + "() int { return " + strconv.Itoa(11+index) + " }"
	}
	contract, err := repositoryfacts.BuildChangeContract(snapshot, analysis, requests)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, analysis, contract, candidates
}

func repositoryPreparationPlanner(
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	contract repositoryfacts.ChangeContract,
	stagedRoots *[]string,
) func(context.Context, []changeapply.DesiredFileState) (*changeapply.StagedChange, error) {
	return func(ctx context.Context, desired []changeapply.DesiredFileState) (*changeapply.StagedChange, error) {
		stage, err := changeapply.PlanFileStateTransitions(ctx, changeapply.FileStateInput{
			Snapshot: snapshot, Analysis: analysis, OwnerID: contract.ID,
			Desired: append([]changeapply.DesiredFileState(nil), desired...),
		})
		if err == nil {
			*stagedRoots = append(*stagedRoots, stage.DeltaRoot())
		}
		return stage, err
	}
}

func planExistingRepositoryTestStage(
	t *testing.T,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	contract repositoryfacts.ChangeContract,
	candidates map[string]string,
) *changeapply.StagedChange {
	t.Helper()
	desired, err := changeapply.AssembleExistingGoFileStates(
		snapshot, analysis, contract, candidates,
	)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: snapshot, Analysis: analysis, OwnerID: contract.ID, Desired: desired,
	})
	if err != nil {
		t.Fatal(err)
	}
	return stage
}

func assertRepositoryPreparationStagesCleaned(t *testing.T, roots []string) {
	t.Helper()
	for _, root := range roots {
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatalf("failed stage %q remains: %v", root, err)
		}
	}
}
