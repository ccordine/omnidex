package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestPathFreeDeletionCandidatesAreDeterministicSafeAndPathBlind(t *testing.T) {
	t.Parallel()
	snapshot, analysis := pathFreeDeletionFixture(t, 2)
	authorities, err := buildPathFreeDeletionCandidateAuthorities(
		context.Background(), snapshot, analysis,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(authorities) != 2 {
		t.Fatalf("candidate count=%d", len(authorities))
	}
	for index, authority := range authorities {
		want := "ARTIFACT_CANDIDATE_" + string(rune('1'+index))
		if authority.input.CandidateID != want {
			t.Fatalf("candidate %d ID=%q want %q", index, authority.input.CandidateID, want)
		}
		if authority.file.Path == "" || len(authority.input.Declarations) != 1 {
			t.Fatalf("candidate authority=%+v", authority)
		}
		if encoded := authority.input.CandidateID + strings.Join(authority.input.Declarations, "\n"); strings.Contains(encoded, authority.file.Path) || strings.Contains(encoded, filepath.Base(authority.file.Path)) {
			t.Fatalf("candidate projection exposed physical identity: %+v", authority)
		}
	}
	if authorities[0].file.Path >= authorities[1].file.Path {
		t.Fatalf("candidates are not deterministically path-sorted code-side: %+v", authorities)
	}
}

func TestPathFreeDeletionCandidatesRejectUnsafeCardinalityBeforeInference(t *testing.T) {
	t.Parallel()
	for _, count := range []int{1, 9} {
		snapshot, analysis := pathFreeDeletionFixture(t, count)
		_, err := buildPathFreeDeletionCandidateAuthorities(t.Context(), snapshot, analysis)
		if err == nil || !strings.Contains(err.Error(), "2-8") {
			t.Fatalf("candidate count %d error=%v", count, err)
		}
	}
}

func TestPathFreeDeletionCandidateEnumerationFailsOnStaleAuthority(t *testing.T) {
	t.Parallel()
	snapshot, analysis := pathFreeDeletionFixture(t, 2)
	if err := os.WriteFile(
		filepath.Join(snapshot.Root, "candidate_01.go"),
		[]byte("package deletionfixture\n\nfunc Candidate01() int { return 99 }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	_, err := buildPathFreeDeletionCandidateAuthorities(t.Context(), snapshot, analysis)
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale authority error=%v", err)
	}
	if _, ineligible := changeapply.DeletionCandidateIneligibilityOf(err); ineligible {
		t.Fatalf("stale authority was silently classified as candidate ineligibility: %v", err)
	}
}

func pathFreeDeletionFixture(t *testing.T, candidates int) (
	repositoryfacts.Snapshot,
	repositoryfacts.Analysis,
) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":           "module example.com/deletionfixture\n\ngo 1.24\n",
		"retained.go":      "package deletionfixture\n\nfunc Retained() int { return UsesRetained() }\n",
		"uses_retained.go": "package deletionfixture\n\nfunc UsesRetained() int { return Retained() }\n",
	}
	for index := 1; index <= candidates; index++ {
		name := fmt.Sprintf("candidate_%02d.go", index)
		files[name] = fmt.Sprintf(
			"package deletionfixture\n\nfunc Candidate%02d() int { return %d }\n", index, index,
		)
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	desiredStateGit(t, root, "init")
	desiredStateGit(t, root, "config", "user.email", "path-free@example.test")
	desiredStateGit(t, root, "config", "user.name", "Path Free")
	desiredStateGit(t, root, "add", ".")
	desiredStateGit(t, root, "commit", "-m", "path-free deletion fixture")
	return desiredStateReindex(t, t.Context(), root)
}
