package version

import (
	"os"
	"strings"
	"testing"
)

func TestBuildCommitAcceptsExactLowercaseGitObjectIdentities(t *testing.T) {
	original := Commit
	t.Cleanup(func() { Commit = original })

	for _, candidate := range []string{
		strings.Repeat("a", 40),
		strings.Repeat("0123456789abcdef", 4),
	} {
		Commit = candidate
		got, err := BuildCommit()
		if err != nil {
			t.Fatalf("BuildCommit(%q): %v", candidate, err)
		}
		if got != candidate {
			t.Fatalf("BuildCommit()=%q want %q", got, candidate)
		}
	}
}

func TestBuildCommitRejectsMissingOrInexactIdentity(t *testing.T) {
	original := Commit
	t.Cleanup(func() { Commit = original })

	for _, candidate := range []string{
		"",
		strings.Repeat("a", 39),
		strings.Repeat("a", 41),
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		strings.Repeat("A", 40),
		strings.Repeat("g", 40),
		" " + strings.Repeat("a", 40),
		strings.Repeat("a", 40) + "\n",
	} {
		Commit = candidate
		if got, err := BuildCommit(); err == nil {
			t.Fatalf("BuildCommit(%q)=%q, want validation error", candidate, got)
		}
	}
}

func TestJSONPreservesExactEmbeddedCommitBytes(t *testing.T) {
	original := Commit
	t.Cleanup(func() { Commit = original })
	Commit = " " + strings.Repeat("a", 40) + " "
	if got := JSON()["commit"]; got != Commit {
		t.Fatalf("JSON commit=%q want exact embedded bytes %q", got, Commit)
	}
	if _, exists := JSON()["migrations_sha256"]; exists {
		t.Fatal("version metadata retained database migration identity")
	}
}

func TestCurrentReleaseIsCharmeleon(t *testing.T) {
	if Version != "v0.5.0" || Codename != "Charmeleon" {
		t.Fatalf("current release=%s %s", Version, Codename)
	}
	if NationalDexID(Codename) != 5 {
		t.Fatalf("Charmeleon National Dex id=%d", NationalDexID(Codename))
	}
	metadata := JSON()
	if metadata["next_maturity_name"] != "Charizard" {
		t.Fatalf("next maturity=%q", metadata["next_maturity_name"])
	}
	if len(PrideLine) < 6 || PrideLine[3].Stage == "current" || PrideLine[4].Stage != "current" {
		t.Fatalf("release stages=%+v", PrideLine)
	}
}

func TestReleaseBuilderDefaultsToCharmeleon(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/build-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	library, err := os.ReadFile("../../scripts/build-release-lib.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw) + "\n" + string(library)
	for _, required := range []string{
		`VERSION="v0.5.0"`, `CODENAME="Charmeleon"`,
		`git -C "$REPO_ROOT" rev-parse HEAD`, `source_archive_sha256`, `git -C "$REPO_ROOT" archive`,
		`internal/version.SourceSHA256`, `copy_database_setup "$target_source" "$target_dir"`,
		`database/setup.sql`,
		`release builds require a clean tracked and untracked worktree`,
		`validate_tracked_release_sources "$REPO_ROOT"`,
		`validate_dist_dir`, `create_dist_dir`, `distribution path enters tracked source`,
		`cd "$target_source"`, `verify_source_stage`, `assert_repository_matches_snapshot`,
		`publish_staged_release`,
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("release builder omitted %s", required)
		}
	}
	for _, forbidden := range []string{
		`cd "$REPO_ROOT"` + "\n" + `      CGO_ENABLED`,
		`cp -a "${REPO_ROOT}/README.md"`,
		`MigrationsSHA256`, `RELEASE_MIGRATIONS_SHA256`, `migrations/SHA256SUMS`,
		`verify_migration_manifest`, `target_source}/migrations`, `write_migration_manifest`,
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("release builder still consumes live source: %s", forbidden)
		}
	}
	if strings.Index(script, `release builds require a clean tracked and untracked worktree`) >
		strings.LastIndex(script, "\n  create_dist_dir\n") {
		t.Fatal("release builder creates its distribution directory before proving a clean worktree")
	}
	if strings.Index(string(raw), `validate_tracked_release_sources "$REPO_ROOT"`) >
		strings.Index(string(raw), "\n  create_dist_dir\n") {
		t.Fatal("release builder creates its distribution directory before rejecting tracked generated artifacts")
	}
	for _, forbidden := range []string{`VERSION="v0.4.0"`, `CODENAME="Charmander"`} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("release builder retained old default %s", forbidden)
		}
	}
}
