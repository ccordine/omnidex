package worker

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestRepositoryShadowSourcesAndSpecsAreStationSpecific(t *testing.T) {
	t.Parallel()
	retrieval, err := assemblyline.NewRepositoryRetrievalJob(
		assemblyline.RepositoryRetrievalInput{ResearchNeed: "Find the exact owner."},
	)
	if err != nil {
		t.Fatal(err)
	}
	retrievalPlan, err := repositoryShadowPlan(retrieval)
	if err != nil {
		t.Fatal(err)
	}
	if len(retrievalPlan.sources) != 1 || retrievalPlan.sources[0].item.Role != workingset.RoleUserAuthority ||
		retrievalPlan.sources[0].material.Authority != taskstate.AuthorityUser || retrievalPlan.spec.MaxItems != 1 {
		t.Fatalf("retrieval plan=%+v", retrievalPlan)
	}

	pack := repositoryProjectionTestPack(t)
	change, err := assemblyline.NewRepositoryChangeSurfaceJob(assemblyline.RepositoryChangeSurfaceInput{
		ResearchNeed: "Find the exact owner.", RequirementQuotes: []string{"exact owner"}, Evidence: pack,
	})
	if err != nil {
		t.Fatal(err)
	}
	changePlan, err := repositoryShadowPlan(change)
	if err != nil {
		t.Fatal(err)
	}
	if len(changePlan.sources) != 2 || changePlan.sources[1].item.Role != workingset.RoleRepositoryEvidence ||
		changePlan.sources[1].material.Authority != taskstate.AuthorityToolEvidence || changePlan.spec.MaxItems != 2 {
		t.Fatalf("change plan=%+v", changePlan)
	}
	if !reflect.DeepEqual(retrievalPlan.sources[0], changePlan.sources[0]) {
		t.Fatal("identical research authority did not resolve to one stable content-addressed item")
	}
	for label, projection := range map[string]string{
		"material": changePlan.sources[1].material.Content,
		"item ID":  string(changePlan.sources[1].item.ID),
		"ref URI":  changePlan.sources[1].item.Ref.URI,
		"ref hash": changePlan.sources[1].item.Ref.Hash,
	} {
		for _, forbidden := range []string{
			"example.test/platform/monorepo/internal/owner",
			"internal/owner/owner.go",
			"owner.go",
			`"qualified_name"`,
			`"file_id"`,
			`"query"`,
		} {
			if strings.Contains(projection, forbidden) {
				t.Fatalf("shadow %s leaked repository identity %q: %s", label, forbidden, projection)
			}
		}
	}
}

func TestRepositoryShadowEligibilityRejectsEveryOtherStation(t *testing.T) {
	t.Parallel()
	for _, kind := range []assemblyline.WorkKind{
		assemblyline.WorkApplicationClassify, assemblyline.WorkRequirementPartition,
		assemblyline.WorkRetrievalBriefing, assemblyline.WorkRetrievalAdvisory,
		assemblyline.WorkFragmentGeneration, assemblyline.WorkFragmentModification,
		assemblyline.WorkFragmentCorrection, assemblyline.WorkResponseCorrection,
	} {
		if repositoryShadowEligible(kind) {
			t.Fatalf("noneligible station %q gained shadow context", kind)
		}
	}
	for _, kind := range []assemblyline.WorkKind{
		assemblyline.WorkRepositoryRetrieval, assemblyline.WorkRepositoryChangeSurface,
	} {
		if !repositoryShadowEligible(kind) {
			t.Fatalf("repository station %q is not eligible", kind)
		}
	}
}

func TestNoneligiblePortableWorkCannotEnterRepositoryShadowPersistence(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "Build a small browser tool."},
	)
	if err != nil {
		t.Fatal(err)
	}
	projectionID, err := prepareRepositoryShadowContext(nil, job)
	if err != nil || projectionID != "" {
		t.Fatalf("noneligible work projection=%q error=%v", projectionID, err)
	}
}

func TestRepositoryShadowSnapshotValidationRejectsWrongBudgetAndStaleItem(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewRepositoryRetrievalJob(
		assemblyline.RepositoryRetrievalInput{ResearchNeed: "Find the exact owner."},
	)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := repositoryShadowPlan(job)
	if err != nil {
		t.Fatal(err)
	}
	owner := workingset.Owner{
		LedgerID: taskstate.LedgerID("ledger_" + strings.Repeat("a", 64)), JobID: 7, Generation: 3,
	}
	wrongBudget, err := workingset.New(owner, workingset.Budget{MaxItems: 1, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRepositoryShadowSnapshot(wrongBudget.Snapshot(), owner, plan.sources); err == nil {
		t.Fatal("existing working set with another budget was accepted")
	}

	set, err := workingset.New(owner, repositoryShadowWorkingSetBudget())
	if err != nil {
		t.Fatal(err)
	}
	request := plan.sources[0].item
	request.Scope = set.Scope()
	request.Ref.Hash = strings.Repeat("f", 64)
	if _, err := set.Acquire(request); err != nil {
		t.Fatal(err)
	}
	if err := validateRepositoryShadowSnapshot(set.Snapshot(), owner, plan.sources); err == nil {
		t.Fatal("stale research authority was accepted")
	}
}

func TestRepositoryShadowRejectsMalformedMaterialBeforeAcquisition(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewRepositoryRetrievalJob(
		assemblyline.RepositoryRetrievalInput{ResearchNeed: "Find\x00owner."},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repositoryShadowPlan(job); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("malformed material error=%v", err)
	}
}

func repositoryProjectionTestPack(t *testing.T) repositoryretrieval.EvidencePack {
	t.Helper()
	binding, err := repositoryretrieval.NewQueryBinding(
		repositoryretrieval.OperationSemanticExcerpts,
		"example.test/platform/monorepo/internal/owner.Owner",
	)
	if err != nil {
		t.Fatal(err)
	}
	pack := repositoryretrieval.EvidencePack{
		Schema:     repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID: "snapshot_" + strings.Repeat("1", 64),
		AnalysisID: "analysis_" + strings.Repeat("2", 64),
		Operation:  repositoryretrieval.OperationSemanticExcerpts, QueryBinding: binding,
		Symbols: []repositoryretrieval.EvidenceSymbol{{
			ID: "symbol_" + strings.Repeat("3", 64), Kind: "function", Name: "Owner",
			Signature: "func Owner()", SourceSHA256: strings.Repeat("4", 64),
		}},
		Relations: []repositoryretrieval.EvidenceRelation{}, SourceOmissions: []repositoryretrieval.SourceOmission{},
		OmittedSymbolIDs: []string{}, MaxBytes: 9 * 1024,
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	return pack
}

func TestRepositoryShadowRuntimeHasNoAppliedOrNoneligibleHook(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "ContextProjectionModeApplied") {
			t.Fatalf("worker %s contains an applied context cutover", path)
		}
	}
	eligibilitySource, err := os.ReadFile("v3_repository_context_sources.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"WorkApplicationClassify", "WorkRequirementPartition", "WorkFragmentGeneration",
		"WorkFragmentModification", "WorkFragmentCorrection", "WorkResponseCorrection",
		"WorkRetrievalBriefing", "WorkRetrievalAdvisory", "WorkRetrievalSynthesis",
	} {
		if strings.Contains(string(eligibilitySource), forbidden) {
			t.Fatalf("repository shadow eligibility source contains noneligible station %q", forbidden)
		}
	}
}
