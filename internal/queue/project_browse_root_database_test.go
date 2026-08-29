package queue

import (
	"path/filepath"
	"testing"
)

func TestFindProjectBrowseRootSelectsOnlyTargetRelatedLocation(t *testing.T) {
	repository, _, ctx := replanTestRepository(t)
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	nested := filepath.Join(root, "nested")
	unrelated := filepath.Join(base, "unrelated")
	for _, location := range []string{root, nested, unrelated} {
		if _, err := repository.pool.Exec(ctx, `
			INSERT INTO projects(location, name) VALUES($1, $2)
		`, location, filepath.Base(location)); err != nil {
			t.Fatal(err)
		}
	}

	got, found, err := repository.FindProjectBrowseRoot(ctx, filepath.Join(nested, "child"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || got != nested {
		t.Fatalf("root=%q found=%t want %q", got, found, nested)
	}

	got, found, err = repository.FindProjectBrowseRoot(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if !found || got == "" {
		t.Fatalf("ancestor lookup root=%q found=%t", got, found)
	}

	_, found, err = repository.FindProjectBrowseRoot(ctx, filepath.Join(base, "repository"))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("prefix-only path must not be authorized as a project root")
	}
}

func TestFindProjectBrowseRootRejectsEmptyTarget(t *testing.T) {
	repository, _, ctx := replanTestRepository(t)
	if _, _, err := repository.FindProjectBrowseRoot(ctx, " "); err == nil {
		t.Fatal("expected empty browse target to fail")
	}
}
