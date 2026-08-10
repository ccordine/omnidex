package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

func TestExistingRepositoryMutationExecutionStopsAtFailedPrepare(t *testing.T) {
	before, contract, commands, _ := repositoryMutationExecutionFixture(t)
	want := errors.New("staged focused proof failed")
	mutations, authoritative, refreshes := 0, 0, 0
	_, err := executeExistingRepositoryMutation(
		context.Background(), contract.ID, commands, before,
		existingRepositoryMutationExecution{
			prepare: func(context.Context) (*verifiedRepositoryChangeStage, error) {
				return nil, want
			},
			mutate: func(context.Context, *verifiedRepositoryChangeStage) error {
				mutations++
				return nil
			},
			verifyAuthoritative: func(context.Context, *verifiedRepositoryChangeStage, []testCommand) error {
				authoritative++
				return nil
			},
			refresh: func(context.Context) (repositoryindex.Result, error) {
				refreshes++
				return repositoryindex.Result{}, nil
			},
		},
	)
	if !errors.Is(err, want) {
		t.Fatalf("prepare failure=%v", err)
	}
	if mutations != 0 || authoritative != 0 || refreshes != 0 {
		t.Fatalf("post-prepare side effects=%d/%d/%d", mutations, authoritative, refreshes)
	}
}

func TestExistingRepositoryMutationExecutionStopsAfterMutationFailure(t *testing.T) {
	before, contract, commands, prepared := repositoryMutationExecutionFixture(t)
	want := errors.New("mutation remained at exact source state")
	authoritative, refreshes := 0, 0
	_, err := executeExistingRepositoryMutation(
		context.Background(), contract.ID, commands, before,
		existingRepositoryMutationExecution{
			prepare: func(context.Context) (*verifiedRepositoryChangeStage, error) {
				return prepared, nil
			},
			mutate: func(context.Context, *verifiedRepositoryChangeStage) error { return want },
			verifyAuthoritative: func(context.Context, *verifiedRepositoryChangeStage, []testCommand) error {
				authoritative++
				return nil
			},
			refresh: func(context.Context) (repositoryindex.Result, error) {
				refreshes++
				return repositoryindex.Result{}, nil
			},
		},
	)
	if !errors.Is(err, want) {
		t.Fatalf("mutation failure=%v", err)
	}
	if authoritative != 0 || refreshes != 0 {
		t.Fatalf("post-mutation work ran after failure: %d/%d", authoritative, refreshes)
	}
	assertRepositoryFixtureSourceValue(t, before.Root)
}

func TestExistingRepositoryMutationExecutionCompletesInExactOrder(t *testing.T) {
	before, contract, commands, prepared := repositoryMutationExecutionFixture(t)
	wantStageID := prepared.ID()
	trace := make([]string, 0, 4)
	result, err := executeExistingRepositoryMutation(
		context.Background(), contract.ID, commands, before,
		existingRepositoryMutationExecution{
			prepare: func(context.Context) (*verifiedRepositoryChangeStage, error) {
				trace = append(trace, "prepare")
				return prepared, nil
			},
			mutate: func(ctx context.Context, stage *verifiedRepositoryChangeStage) error {
				trace = append(trace, "mutate")
				_, applyErr := stage.ApplyVerified(ctx)
				return applyErr
			},
			verifyAuthoritative: func(
				_ context.Context, _ *verifiedRepositoryChangeStage, exact []testCommand,
			) error {
				trace = append(trace, "authoritative")
				if !reflect.DeepEqual(exact, commands) {
					t.Fatalf("authoritative plan differs from staged plan")
				}
				return nil
			},
			refresh: func(context.Context) (repositoryindex.Result, error) {
				trace = append(trace, "refresh")
				return existingRepositoryRefreshedIndex(t, before.Root), nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(trace, []string{"prepare", "mutate", "authoritative", "refresh"}) {
		t.Fatalf("execution order=%v", trace)
	}
	if result.StageID != wantStageID || len(result.ChangedFileIDs) != 1 ||
		!result.Refreshed.Complete || result.Refreshed.Snapshot.ID == before.ID {
		t.Fatalf("mutation result=%+v", result)
	}
	if _, err := prepared.ApplyVerified(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "closed") {
		t.Fatalf("verified stage was not cleaned exactly at execution exit: %v", err)
	}
}

func TestExistingRepositoryMutationExecutionPreservesAppliedTruthOnProofFailure(t *testing.T) {
	before, contract, commands, prepared := repositoryMutationExecutionFixture(t)
	want := errors.New("authoritative broad proof failed")
	journalApplied, refreshed := false, false
	_, err := executeExistingRepositoryMutation(
		context.Background(), contract.ID, commands, before,
		existingRepositoryMutationExecution{
			prepare: func(context.Context) (*verifiedRepositoryChangeStage, error) {
				return prepared, nil
			},
			mutate: func(ctx context.Context, stage *verifiedRepositoryChangeStage) error {
				if _, err := stage.ApplyVerified(ctx); err != nil {
					return err
				}
				journalApplied = true
				return nil
			},
			verifyAuthoritative: func(context.Context, *verifiedRepositoryChangeStage, []testCommand) error {
				return want
			},
			refresh: func(context.Context) (repositoryindex.Result, error) {
				refreshed = true
				return existingRepositoryRefreshedIndex(t, before.Root), nil
			},
		},
	)
	if !errors.Is(err, want) || !journalApplied || !refreshed {
		t.Fatalf("proof failure=%v applied=%t refreshed=%t", err, journalApplied, refreshed)
	}
	assertRepositoryFixturePostValue(t, before.Root)
}

func TestExistingRepositoryMutationExecutionPreservesAppliedTruthOnRefreshFailure(t *testing.T) {
	before, contract, commands, prepared := repositoryMutationExecutionFixture(t)
	want := errors.New("repository reindex failed")
	journalApplied := false
	_, err := executeExistingRepositoryMutation(
		context.Background(), contract.ID, commands, before,
		existingRepositoryMutationExecution{
			prepare: func(context.Context) (*verifiedRepositoryChangeStage, error) {
				return prepared, nil
			},
			mutate: func(ctx context.Context, stage *verifiedRepositoryChangeStage) error {
				_, applyErr := stage.ApplyVerified(ctx)
				journalApplied = applyErr == nil
				return applyErr
			},
			verifyAuthoritative: func(context.Context, *verifiedRepositoryChangeStage, []testCommand) error { return nil },
			refresh: func(context.Context) (repositoryindex.Result, error) {
				return repositoryindex.Result{}, want
			},
		},
	)
	if !errors.Is(err, want) || !journalApplied {
		t.Fatalf("refresh failure=%v applied=%t", err, journalApplied)
	}
	assertRepositoryFixturePostValue(t, before.Root)
}

func TestExistingRepositoryMutationExecutionRequiresExactFinalTargetHash(t *testing.T) {
	before, contract, commands, prepared := repositoryMutationExecutionFixture(t)
	_, err := executeExistingRepositoryMutation(
		context.Background(), contract.ID, commands, before,
		existingRepositoryMutationExecution{
			prepare: func(context.Context) (*verifiedRepositoryChangeStage, error) { return prepared, nil },
			mutate: func(ctx context.Context, stage *verifiedRepositoryChangeStage) error {
				_, applyErr := stage.ApplyVerified(ctx)
				return applyErr
			},
			verifyAuthoritative: func(context.Context, *verifiedRepositoryChangeStage, []testCommand) error { return nil },
			refresh: func(context.Context) (repositoryindex.Result, error) {
				if err := os.WriteFile(filepath.Join(before.Root, "first.go"), []byte(
					"package verification\n\nfunc First() int { return 9 }\n",
				), 0o600); err != nil {
					t.Fatal(err)
				}
				return existingRepositoryRefreshedIndex(t, before.Root), nil
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "exact contract") {
		t.Fatalf("wrong final target hash error=%v", err)
	}
}

func repositoryMutationExecutionFixture(
	t *testing.T,
) (repositoryfacts.Snapshot, repositoryfacts.ChangeContract, []testCommand, *verifiedRepositoryChangeStage) {
	t.Helper()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	target := existingRepositoryVerificationSymbol(t, analysis, "First")
	contract, err := repositoryfacts.BuildChangeContract(snapshot, analysis, []repositoryfacts.ChangeRequest{{
		SymbolID: target.ID, RequirementQuote: "Preserve behavior while changing the declaration.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := changeapply.Plan(t.Context(), changeapply.Input{
		Snapshot: snapshot, Analysis: analysis, Contract: contract,
		Candidates: []changeapply.CandidateDeclaration{{
			SymbolID: target.ID, Declaration: "func First() int { return 1 + 0 }",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := newVerifiedRepositoryChangeStage(contract.ID, commands, stage)
	if err != nil {
		_ = stage.Cleanup()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Cleanup() })
	return snapshot, contract, commands, prepared
}

func assertRepositoryFixtureSourceValue(t *testing.T, root string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "first.go"))
	if err != nil || strings.Contains(string(raw), "return 1 + 0") {
		t.Fatalf("repository source changed after failed mutation: %q error=%v", raw, err)
	}
}

func assertRepositoryFixturePostValue(t *testing.T, root string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "first.go"))
	if err != nil || !strings.Contains(string(raw), "return 1 + 0") {
		t.Fatalf("repository lost exact applied bytes: %q error=%v", raw, err)
	}
}
