package worker

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

func TestPostgresRepositoryBaselineRejectsConflictingExistingTestBeforeGenerationOrMutation(t *testing.T) {
	if os.Getenv("OMNIDEX_REQUIRE_BWRAP_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_REQUIRE_BWRAP_INTEGRATION=1 for the real dirty-baseline proof")
	}
	ctx, repository, pool := openRepositoryTestDatabase(t)
	root := repositoryConflictingBaselineRoot(t)
	project, err := repository.CreateProject(
		ctx, fmt.Sprintf("mutation-baseline-failure-%d", time.Now().UnixNano()), root, "", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	indexer, err := repositoryindex.New(repository)
	if err != nil {
		t.Fatal(err)
	}
	before, err := indexer.Refresh(ctx, project.ID, root)
	if err != nil {
		t.Fatal(err)
	}
	analysis := before.Analyses[0]
	target := existingRepositoryVerificationSymbol(t, analysis, "Value")
	contract, err := repositoryfacts.BuildChangeContract(
		before.Snapshot, analysis,
		[]repositoryfacts.ChangeRequest{{
			SymbolID: target.ID, RequirementQuote: "Return the independently requested value.",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := existingRepositoryGoVerificationCommands(before.Snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	claim := claimRepositoryMutationWorkflowJob(t, ctx, repository, project.ID, root)
	session := &directCodingSession{
		runtime: &nativeRuntimeV3{
			svc: &Service{
				repo: repository, repositoryIndex: indexer,
				logger: log.New(io.Discard, "", 0),
			},
			ctx: ctx, claim: claim, contexts: make(map[string]string),
		},
		root: root, repositoryIndex: &before,
	}
	baseline, err := session.proveExistingRepositoryBaseline(
		before.Snapshot, contract.ID, commands,
	)
	if err == nil || baseline != nil || !strings.Contains(err.Error(), "repository verification") {
		t.Fatalf("dirty baseline result=%+v error=%v", baseline, err)
	}

	var operations, generatedDiffs, calls, acceptances int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM repository_mutation_operations WHERE job_id=$1
	`, claim.Job.ID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT
		  COUNT(*) FILTER (WHERE kind=$2 AND source_type='repository'),
		  COUNT(*) FILTER (WHERE COALESCE((payload_json->'metadata'->>'repository_verification_baseline_accepted')::boolean,false))
		FROM evidence WHERE job_id=$1
	`, claim.Job.ID, evidence.KindGeneratedDiff).Scan(&generatedDiffs, &acceptances); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM llm_call_evidence WHERE job_id=$1
	`, claim.Job.ID).Scan(&calls); err != nil {
		t.Fatal(err)
	}
	if operations != 0 || generatedDiffs != 0 || calls != 0 || acceptances != 0 {
		t.Fatalf(
			"dirty baseline escaped authority: operations=%d diffs=%d calls=%d acceptances=%d",
			operations, generatedDiffs, calls, acceptances,
		)
	}
	raw, readErr := os.ReadFile(filepath.Join(root, "value.go"))
	if readErr != nil || string(raw) != "package mutationworkflow\n\nfunc Value() int { return 1 }\n" {
		t.Fatalf("dirty baseline changed source: %q error=%v", raw, readErr)
	}
}

func TestPostgresRepositoryBaselineFailsLoudlyWhenTestRequiresIgnoredSecretState(t *testing.T) {
	if os.Getenv("OMNIDEX_REQUIRE_BWRAP_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_REQUIRE_BWRAP_INTEGRATION=1 for the real excluded-state proof")
	}
	ctx, repository, pool := openRepositoryTestDatabase(t)
	root := repositoryMutationWorkflowRoot(t)
	requiresSecret := `package mutationworkflow

import (
	"fmt"
	"os"
	"testing"
)

func TestValue(t *testing.T) {
	if Value() == 0 { t.Fatal("zero value") }
	content, err := os.ReadFile(".env")
	if err != nil { t.Fatal("ignored secret state is unavailable") }
	fmt.Printf("LEAKED_SECRET=%s", content)
}
`
	if err := os.WriteFile(filepath.Join(root, "value_test.go"), []byte(requiresSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		ctx, fmt.Sprintf("mutation-excluded-state-%d", time.Now().UnixNano()), root, "", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	indexer, err := repositoryindex.New(repository)
	if err != nil {
		t.Fatal(err)
	}
	before, err := indexer.Refresh(ctx, project.ID, root)
	if err != nil {
		t.Fatal(err)
	}
	analysis := before.Analyses[0]
	target := existingRepositoryVerificationSymbol(t, analysis, "Value")
	contract, err := repositoryfacts.BuildChangeContract(
		before.Snapshot, analysis, []repositoryfacts.ChangeRequest{{
			SymbolID: target.ID, RequirementQuote: "Preserve exact source behavior.",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := existingRepositoryGoVerificationCommands(before.Snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	claim := claimRepositoryMutationWorkflowJob(t, ctx, repository, project.ID, root)
	session := &directCodingSession{
		runtime: &nativeRuntimeV3{
			svc: &Service{
				repo: repository, repositoryIndex: indexer,
				logger: log.New(io.Discard, "", 0),
			},
			ctx: ctx, claim: claim, contexts: make(map[string]string),
		},
		root: root, repositoryIndex: &before,
	}
	baseline, err := session.proveExistingRepositoryBaseline(
		before.Snapshot, contract.ID, commands,
	)
	if err == nil || baseline != nil {
		t.Fatalf("excluded-state-dependent test produced baseline=%+v error=%v", baseline, err)
	}
	for _, canary := range repositoryMutationSecretCanaries {
		var leaked int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM evidence
			WHERE job_id=$1 AND payload_json::text LIKE '%' || $2 || '%'
		`, claim.Job.ID, canary).Scan(&leaked); err != nil {
			t.Fatal(err)
		}
		if leaked != 0 {
			t.Fatalf("failed excluded-state proof leaked canary %q", canary)
		}
	}
	var calls, operations, acceptances int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM llm_call_evidence WHERE job_id=$1`, claim.Job.ID).Scan(&calls); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM repository_mutation_operations WHERE job_id=$1`, claim.Job.ID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM evidence WHERE job_id=$1
		AND COALESCE((payload_json->'metadata'->>'repository_verification_baseline_accepted')::boolean,false)
	`, claim.Job.ID).Scan(&acceptances); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || operations != 0 || acceptances != 0 {
		t.Fatalf("excluded-state proof escaped: calls=%d operations=%d acceptances=%d", calls, operations, acceptances)
	}
}

func repositoryConflictingBaselineRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":        "module example.com/mutationworkflow\n\ngo 1.24\n",
		"value.go":      "package mutationworkflow\n\nfunc Value() int { return 1 }\n",
		"value_test.go": "package mutationworkflow\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) { if Value() != 2 { t.Fatal(\"wrong value\") } }\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "baseline@example.test"},
		{"config", "user.name", "Baseline Test"}, {"add", "."}, {"commit", "-m", "source"},
	} {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	return root
}
