package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

func TestExistingRepositoryChangeContractRequiresExactIndexedSnapshotAndAnalysis(t *testing.T) {
	t.Parallel()
	pack := repositoryProjectionTestPack(t)
	surface := assemblyline.RepositoryChangeSurfaceDecision{
		Schema: assemblyline.RepositoryChangeSurfaceSchemaV1,
		Targets: []assemblyline.RepositoryChangeTarget{{
			SymbolID: pack.Symbols[0].ID, RequirementQuote: "exact owner",
		}},
		UnresolvedRequirementQuotes: []string{},
	}
	session := &directCodingSession{repositoryIndex: &repositoryindex.Result{
		Snapshot: repositoryfacts.Snapshot{ID: "snapshot_" + strings.Repeat("f", 64)},
	}}
	if _, err := session.buildExistingRepositoryChangeContract(pack, surface); err == nil ||
		!strings.Contains(err.Error(), "differs from current index") {
		t.Fatalf("snapshot mismatch error=%v", err)
	}
	session.repositoryIndex.Snapshot.ID = pack.SnapshotID
	if _, err := session.buildExistingRepositoryChangeContract(pack, surface); err == nil ||
		!strings.Contains(err.Error(), "absent from the immutable index") {
		t.Fatalf("analysis mismatch error=%v", err)
	}
}
