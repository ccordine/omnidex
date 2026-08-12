package repositoryobjective

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func newCommittedGoFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for repository-objective tests")
	}
	root := t.TempDir()
	runFixtureGit(t, root, "init", "--quiet")
	runFixtureGit(t, root, "config", "user.email", "repository-objective@example.test")
	runFixtureGit(t, root, "config", "user.name", "Repository Objective Test")
	paths := make([]string, 0, len(files))
	for relative, content := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	runFixtureGit(t, root, append([]string{"add", "--"}, paths...)...)
	runFixtureGit(t, root, "commit", "--quiet", "-m", "committed fixture")
	return root
}

func runFixtureGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", append([]string{"-C", root}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
	}
}

func fullAcceptance() []AcceptancePredicate {
	return []AcceptancePredicate{
		AcceptanceSubjectResolved,
		AcceptanceDeclarationObserved,
		AcceptanceDirectRelationsKnown,
		AcceptanceApplicableTestsKnown,
	}
}

func deliveryFixture(t *testing.T) string {
	t.Helper()
	return newCommittedGoFixture(t, map[string]string{
		"go.mod": "module example.test/delivery\n\ngo 1.24\n",
		"delivery.go": `package delivery

type ClientConfig struct { Interval int }

func nextWindow(config ClientConfig) int { return config.Interval }

func Dispatch(config ClientConfig) int { return nextWindow(config) }

func Run(config ClientConfig) int { return Dispatch(config) }
`,
		"delivery_test.go": `package delivery

import "testing"

func TestDispatch(t *testing.T) {
	config := ClientConfig{Interval: 7}
	if Dispatch(config) != 7 || Run(config) != 7 { t.Fatal("unexpected window") }
}
`,
	})
}

func storageFixture(t *testing.T) string {
	t.Helper()
	return newCommittedGoFixture(t, map[string]string{
		"go.mod": "module example.test/storage\n\ngo 1.24\n",
		"cache/cache.go": `package cache

func memoryEntry(key string) string { return "memory:" + key }

func Resolve(key string) string { return memoryEntry(key) }
`,
		"cache/cache_test.go": `package cache

import "testing"

func TestResolve(t *testing.T) {
	if Resolve("a") == "" { t.Fatal("empty") }
}
`,
		"database/database.go": `package database

func durableRecord(key string) string { return "durable:" + key }

func Resolve(key string) string { return durableRecord(key) }
`,
		"database/database_test.go": `package database

import "testing"

func TestResolve(t *testing.T) {
	if Resolve("a") == "" { t.Fatal("empty") }
}
`,
	})
}

type selectorFunc func(context.Context, SemanticGap) (CandidateID, error)

func (function selectorFunc) Select(ctx context.Context, gap SemanticGap) (CandidateID, error) {
	return function(ctx, gap)
}
