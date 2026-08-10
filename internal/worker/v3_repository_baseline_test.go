package worker

import (
	"os"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestRepositoryBaselineAuthorityBindsSourceContractAndOrderedPlan(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	target := existingRepositoryVerificationSymbol(t, analysis, "First")
	contract, err := repositoryfacts.BuildChangeContract(
		snapshot, analysis, []repositoryfacts.ChangeRequest{{
			SymbolID: target.ID, RequirementQuote: "change First",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := newRepositoryBaselineVerificationAuthority(
		snapshot.ID, contract.ID, commands,
	)
	if err != nil {
		t.Fatal(err)
	}
	metadata := authority.metadata()
	if metadata["repository_verification_baseline_id"] != authority.baselineID ||
		metadata["repository_source_snapshot_id"] != snapshot.ID ||
		metadata["repository_change_contract_id"] != contract.ID ||
		metadata["repository_verification_plan_id"] != authority.planID {
		t.Fatalf("baseline metadata=%#v authority=%+v", metadata, authority)
	}
	for _, forbidden := range []string{
		"repository_change_stage_id", "repository_change_patch_sha256", "repository_expected_post_id",
	} {
		if _, exists := metadata[forbidden]; exists {
			t.Fatalf("baseline authority fabricated post-change identity %q: %#v", forbidden, metadata)
		}
	}
	tampered := cloneTestCommands(commands)
	tampered[0].Args[4] = "^TestOther$"
	if err := authority.validate(tampered); err == nil {
		t.Fatal("baseline authority accepted a different ordered verification plan")
	}
}

func TestRepositoryBaselineAcceptanceIsDistinctFromPostChangeAcceptance(t *testing.T) {
	t.Parallel()
	authority := repositoryBaselineVerificationAuthority{
		baselineID: "repository_baseline_one", sourceSnapshotID: "snapshot-one",
		contractID: "contract-one", planID: "plan-one",
	}
	record := repositoryVerificationAcceptanceEvidence(
		authority, repositoryVerificationBaseline, []testCommand{{}, {}},
	)
	if record.Metadata["repository_verification_baseline_accepted"] != true ||
		record.Metadata["repository_verification_scope"] != "baseline" ||
		record.Metadata["repository_verification_command_count"] != 2 ||
		record.SourceType != "command-baseline" {
		t.Fatalf("baseline acceptance evidence=%#v record=%+v", record.Metadata, record)
	}
	if _, exists := record.Metadata["repository_verification_plan_accepted"]; exists {
		t.Fatalf("baseline evidence claimed post-change acceptance: %#v", record.Metadata)
	}
}

func TestRepositoryBaselinePrecedesGenerationAndHasNoPostChangeAuthority(t *testing.T) {
	t.Parallel()
	workflow := readRepositoryBaselineSource(t, "v3_existing_repository_workflow.go")
	derive := strings.Index(workflow, "existingRepositoryGoVerificationCommands(")
	baseline := strings.Index(workflow, "proveExistingRepositoryBaseline(")
	generation := strings.Index(workflow, "generateExistingRepositoryChangeCandidates(")
	mutation := strings.Index(workflow, "applyExistingRepositoryChangeContract(")
	if derive < 0 || baseline < derive || generation < baseline || mutation < generation {
		t.Fatalf(
			"repository workflow order is not contract -> plan -> baseline -> generation -> mutation: %d/%d/%d/%d",
			derive, baseline, generation, mutation,
		)
	}

	baselineSource := readRepositoryBaselineSource(t, "v3_repository_baseline.go")
	for _, forbidden := range []string{
		"changeapply.Plan", "stageID", "patchSHA256", "expectedPost",
		"generateExistingRepositoryChangeCandidates", "applyExistingRepositoryChangeContract",
		"prepareVerifiedExistingRepositoryChange", "executeExistingRepositoryMutation",
	} {
		if strings.Contains(baselineSource, forbidden) {
			t.Fatalf("repository baseline source fabricates or executes post-change authority %q", forbidden)
		}
	}

	generationSource := readRepositoryBaselineSource(t, "v3_repository_change_generation.go")
	if require := strings.Index(generationSource, "baseline.RequireAuthority("); require < 0 || require > strings.Index(generationSource, "session.workerModel(") {
		t.Fatal("repository generation is not gated by exact accepted baseline authority")
	}
	applySource := readRepositoryBaselineSource(t, "v3_repository_change_apply.go")
	if require := strings.Index(applySource, "baseline.RequireAuthority("); require < 0 || require > strings.Index(applySource, "executeExistingRepositoryMutation(") {
		t.Fatal("repository mutation is not gated by exact accepted baseline authority")
	}
}

func readRepositoryBaselineSource(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
