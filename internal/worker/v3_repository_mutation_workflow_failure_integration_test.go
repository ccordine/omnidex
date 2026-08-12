package worker

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

func TestPostgresRepositoryMutationWorkflowRejectsFailedStagedProofBeforeMutation(t *testing.T) {
	if os.Getenv("OMNIDEX_REQUIRE_BWRAP_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_REQUIRE_BWRAP_INTEGRATION=1 for the real failed staged proof")
	}
	ctx, repository, pool := openRepositoryTestDatabase(t)
	root := repositoryMutationWorkflowRoot(t)
	project, err := repository.CreateProject(
		ctx, fmt.Sprintf("mutation-workflow-failure-%d", time.Now().UnixNano()), root, "", "", nil,
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
		[]repositoryfacts.ChangeRequest{{SymbolID: target.ID, RequirementQuote: "Return the verified value."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := existingRepositoryGoVerificationCommands(before.Snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	claim := claimRepositoryMutationWorkflowJob(t, ctx, repository, project.ID, root)
	service := &Service{
		repo: repository, repositoryIndex: indexer,
		logger: log.New(io.Discard, "", 0),
	}
	session := &directCodingSession{
		runtime: &nativeRuntimeV3{
			svc: service, ctx: ctx, claim: claim, contexts: make(map[string]string),
		},
		root: root, repositoryIndex: &before,
	}
	baseline, err := session.proveExistingRepositoryBaseline(
		before.Snapshot, contract.ID, commands,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.applyExistingRepositoryChangeContract(
		contract, map[string]string{target.ID: "func Value() int { return 0 }"}, baseline,
	)
	if err == nil {
		t.Fatal("failed staged focused proof reached repository mutation success")
	}
	raw, readErr := os.ReadFile(filepath.Join(root, "value.go"))
	if readErr != nil || strings.Contains(string(raw), "return 0") ||
		string(raw) != "package mutationworkflow\n\nfunc Value() int { return 1 }\n" {
		t.Fatalf("source changed after staged proof failure: %q error=%v", raw, readErr)
	}
	var operations, generatedDiffs, acceptances, baselineAcceptances, failedStagedProofs, indexEvidence int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM repository_mutation_operations WHERE job_id=$1
	`, claim.Job.ID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT
		  COUNT(*) FILTER (WHERE kind=$2 AND source_type='repository'),
		  COUNT(*) FILTER (WHERE COALESCE((payload_json->'metadata'->>'repository_verification_plan_accepted')::boolean,false)),
		  COUNT(*) FILTER (WHERE COALESCE((payload_json->'metadata'->>'repository_verification_baseline_accepted')::boolean,false)),
		  COUNT(*) FILTER (WHERE kind=$3 AND payload_json->'metadata'->>'repository_verification_scope'='staged'
		    AND payload_json->'metadata'->>'repository_structured_proof_valid'='false'),
		  COUNT(*) FILTER (WHERE kind=$4)
		FROM evidence WHERE job_id=$1
	`, claim.Job.ID, evidence.KindGeneratedDiff, evidence.KindTestResult,
		evidence.KindRepositoryIndex).Scan(
		&generatedDiffs, &acceptances, &baselineAcceptances, &failedStagedProofs, &indexEvidence,
	); err != nil {
		t.Fatal(err)
	}
	if operations != 0 || generatedDiffs != 0 || acceptances != 0 ||
		baselineAcceptances != 1 || indexEvidence != 0 || failedStagedProofs != 1 {
		t.Fatalf(
			"failed staged proof side effects: operations=%d diff=%d acceptance=%d baseline=%d failed=%d refresh=%d",
			operations, generatedDiffs, acceptances, baselineAcceptances, failedStagedProofs, indexEvidence,
		)
	}
}
