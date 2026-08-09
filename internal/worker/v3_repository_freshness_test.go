package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

func TestRepositoryFreshnessRejectsChangedAuthorityBeforeModelProjection(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	session := &directCodingSession{
		runtime: &nativeRuntimeV3{ctx: context.Background()}, root: snapshot.Root,
		repositoryIndex: &repositoryindex.Result{
			Snapshot: snapshot, Analyses: []repositoryfacts.Analysis{analysis}, Complete: true,
		},
	}
	if err := session.requireCurrentRepositoryAuthority("change-surface projection"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(snapshot.Root, "first.go"),
		[]byte("package verification\n\nfunc First() int { return 42 }\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := session.requireCurrentRepositoryAuthority("fragment projection"); err == nil ||
		!strings.Contains(err.Error(), "stale_repository_authority") {
		t.Fatalf("stale repository error=%v", err)
	}
}
