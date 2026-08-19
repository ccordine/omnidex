package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestExistingRepositoryChangeContractRequiresExactIndexedSnapshotAndAnalysis(t *testing.T) {
	t.Parallel()
	pack := repositoryProjectionTestPack(t)
	resolutions := []existingRepositoryRequirementResolution{repositoryRequirementResolution(
		pack, "example.test/platform/monorepo/internal/owner.Owner", "exact owner", pack.Symbols[0].ID,
	)}
	session := &directCodingSession{repositoryIndex: &repositoryindex.Result{
		Snapshot: repositoryfacts.Snapshot{ID: "snapshot_" + strings.Repeat("f", 64)},
	}}
	if _, err := session.buildExistingRepositoryChangeContract(resolutions); err == nil ||
		!strings.Contains(err.Error(), "differs from current index") {
		t.Fatalf("snapshot mismatch error=%v", err)
	}
	session.repositoryIndex.Snapshot.ID = pack.SnapshotID
	if _, err := session.buildExistingRepositoryChangeContract(resolutions); err == nil ||
		!strings.Contains(err.Error(), "absent from the immutable index") {
		t.Fatalf("analysis mismatch error=%v", err)
	}
}

func TestExistingRepositoryChangeContractMergesExactRequirementMappings(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	second := existingRepositoryVerificationSymbol(t, analysis, "Second")
	firstRequirement, secondRequirement := "change First", "change Second"
	firstPack := repositoryRequirementPack(t, snapshot.ID, analysis.ID, firstRequirement, first)
	secondPack := repositoryRequirementPack(t, snapshot.ID, analysis.ID, secondRequirement, second)
	resolutions := []existingRepositoryRequirementResolution{
		repositoryRequirementResolution(firstPack, firstRequirement, firstRequirement, first.ID),
		repositoryRequirementResolution(secondPack, secondRequirement, secondRequirement, second.ID),
	}
	session := &directCodingSession{repositoryIndex: &repositoryindex.Result{
		Snapshot: snapshot, Analyses: []repositoryfacts.Analysis{analysis},
	}}
	contract, err := session.buildExistingRepositoryChangeContract(resolutions)
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.Validate(snapshot, analysis); err != nil {
		t.Fatalf("merged contract is not exact: %v", err)
	}
	got := make(map[string]string, len(contract.Targets))
	for _, target := range contract.Targets {
		got[target.SymbolID] = target.RequirementQuote
	}
	if got[first.ID] != firstRequirement || got[second.ID] != secondRequirement || len(got) != 2 {
		t.Fatalf("merged target mappings=%v", got)
	}
}

func TestExistingRepositoryChangeContractRejectsCrossRequirementSymbolCollision(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	firstRequirement, secondRequirement := "change First", "also change First"
	firstPack := repositoryRequirementPack(t, snapshot.ID, analysis.ID, firstRequirement, first)
	secondPack := repositoryRequirementPack(t, snapshot.ID, analysis.ID, secondRequirement, first)
	resolutions := []existingRepositoryRequirementResolution{
		repositoryRequirementResolution(firstPack, firstRequirement, firstRequirement, first.ID),
		repositoryRequirementResolution(secondPack, secondRequirement, secondRequirement, first.ID),
	}
	session := &directCodingSession{repositoryIndex: &repositoryindex.Result{
		Snapshot: snapshot, Analyses: []repositoryfacts.Analysis{analysis},
	}}
	if _, err := session.buildExistingRepositoryChangeContract(resolutions); err == nil ||
		!strings.Contains(err.Error(), "multi-requirement targets are unsupported") {
		t.Fatalf("cross-requirement symbol collision error=%v", err)
	}
}

func TestExistingRepositoryChangeContractRequiresOneSnapshotAndAnalysisAcrossRequirements(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	second := existingRepositoryVerificationSymbol(t, analysis, "Second")
	firstRequirement, secondRequirement := "change First", "change Second"
	firstPack := repositoryRequirementPack(t, snapshot.ID, analysis.ID, firstRequirement, first)
	secondPack := repositoryRequirementPack(
		t, "snapshot_"+strings.Repeat("9", 64), analysis.ID, secondRequirement, second,
	)
	resolutions := []existingRepositoryRequirementResolution{
		repositoryRequirementResolution(firstPack, firstRequirement, firstRequirement, first.ID),
		repositoryRequirementResolution(secondPack, secondRequirement, secondRequirement, second.ID),
	}
	session := &directCodingSession{repositoryIndex: &repositoryindex.Result{
		Snapshot: snapshot, Analyses: []repositoryfacts.Analysis{analysis},
	}}
	if _, err := session.buildExistingRepositoryChangeContract(resolutions); err == nil ||
		!strings.Contains(err.Error(), "share one snapshot and analysis") {
		t.Fatalf("cross-authority merge error=%v", err)
	}
}

func repositoryRequirementResolution(
	pack repositoryretrieval.EvidencePack,
	query, requirementQuote, symbolID string,
) existingRepositoryRequirementResolution {
	return existingRepositoryRequirementResolution{
		Acquisition: existingRepositoryEvidenceAcquisition{
			RequirementQuote: requirementQuote, Query: query, Pack: pack,
		},
		Surface: assemblyline.RepositoryChangeSurfaceDecision{
			Schema: assemblyline.RepositoryChangeSurfaceSchemaV2,
			Targets: []assemblyline.RepositoryChangeTarget{{
				SymbolID: symbolID, Requirement: requirementQuote,
			}},
		},
	}
}

func repositoryRequirementPack(
	t *testing.T,
	snapshotID, analysisID, query string,
	symbol repositoryfacts.Symbol,
) repositoryretrieval.EvidencePack {
	t.Helper()
	binding, err := repositoryretrieval.NewQueryBinding(
		repositoryretrieval.OperationSemanticExcerpts, query,
	)
	if err != nil {
		t.Fatal(err)
	}
	pack := repositoryretrieval.EvidencePack{
		Schema: repositoryretrieval.EvidencePackSchemaV2, SnapshotID: snapshotID, AnalysisID: analysisID,
		Operation: repositoryretrieval.OperationSemanticExcerpts, QueryBinding: binding,
		Symbols: []repositoryretrieval.EvidenceSymbol{{
			ID: symbol.ID, Kind: symbol.Kind, Name: symbol.Name,
			Signature: symbol.Signature, SourceSHA256: symbol.SourceSHA256,
		}},
		Relations: []repositoryretrieval.EvidenceRelation{}, SourceOmissions: []repositoryretrieval.SourceOmission{},
		OmittedSymbolIDs: []string{}, MaxBytes: 9 * 1024,
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	return pack
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
		Schema:       repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID:   "snapshot_" + strings.Repeat("1", 64),
		AnalysisID:   "analysis_" + strings.Repeat("2", 64),
		Operation:    repositoryretrieval.OperationSemanticExcerpts,
		QueryBinding: binding,
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
