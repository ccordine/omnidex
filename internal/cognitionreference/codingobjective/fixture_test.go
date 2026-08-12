package codingobjective

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

type codingFixture struct {
	root             string
	target           string
	requirement      string
	directCapability string
	candidate        string
	forbidden        []string
}

type workspaceEntry struct {
	path    string
	mode    os.FileMode
	content []byte
	link    string
}

type exactWorkspaceState struct {
	entries   []workspaceEntry
	gitStatus []byte
}

func newCodingFixture(t *testing.T, files map[string]string, fixture codingFixture) codingFixture {
	t.Helper()
	fixture.root = t.TempDir()
	for relative, content := range files {
		absolute := filepath.Join(fixture.root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, arguments := range [][]string{
		{"init"},
		{"config", "user.email", "reference@example.test"},
		{"config", "user.name", "Omnidex Reference"},
		{"add", "."},
		{"commit", "-m", "fixture"},
	} {
		command := exec.Command("git", append([]string{"-C", fixture.root}, arguments...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
	}
	return fixture
}

func numericCodingFixture(t *testing.T) codingFixture {
	t.Helper()
	return newCodingFixture(t, map[string]string{
		"go.mod": "module example.com/numericfixture\n\ngo 1.24\n",
		"fee.go": `package fee

func baseFee() int { return 7 }

func Fee() int { return baseFee() + 1 }
`,
		"fee_test.go": `package fee

import "testing"

func TestFee(t *testing.T) {
	if got := Fee(); got != 7 {
		t.Fatalf("unexpected numeric fee: %d", got)
	}
}
`,
	}, codingFixture{
		target:           "example.com/numericfixture.Fee",
		requirement:      "Fee returns exactly the base fee value.",
		directCapability: "baseFee",
		candidate:        "func Fee() int { return baseFee() }",
		forbidden: []string{
			"fee.go", "fee_test.go", "go.mod", "example.com/numericfixture",
			"unexpected numeric fee", "go test", "gofmt", "git",
		},
	})
}

func presentationCodingFixture(t *testing.T) codingFixture {
	t.Helper()
	return newCodingFixture(t, map[string]string{
		"go.mod": "module example.com/presentationfixture\n\ngo 1.24\n",
		"banner.go": `package banner

func decorate(name string) string { return "Hello, " + name }

func Banner(name string) string { return decorate(name) + "!" }
`,
		"banner_test.go": `package banner

import "testing"

func TestBanner(t *testing.T) {
	if got := Banner("Ada"); got != "Hello, Ada" {
		t.Fatalf("unexpected presentation banner: %q", got)
	}
}
`,
	}, codingFixture{
		target:           "example.com/presentationfixture.Banner",
		requirement:      "Banner returns exactly the decorated presentation.",
		directCapability: "decorate",
		candidate:        "func Banner(name string) string { return decorate(name) }",
		forbidden: []string{
			"banner.go", "banner_test.go", "go.mod", "example.com/presentationfixture",
			"unexpected presentation banner", "go test", "gofmt", "git",
		},
	})
}

func fixtureObjective(fixture codingFixture) Objective {
	return Objective{
		ID: "objective.reference-existing-go-change", Root: fixture.root,
		Target: fixture.target, RequirementQuote: fixture.requirement,
		Acceptance: []AcceptancePredicate{AcceptanceGoTestsPass},
	}
}

func snapshotFixture(t *testing.T, root string) repositoryfacts.Snapshot {
	t.Helper()
	snapshot, err := repositoryfacts.BuildGitSnapshot(
		context.Background(), root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func captureExactWorkspace(t *testing.T, root string) exactWorkspaceState {
	t.Helper()
	state := exactWorkspaceState{entries: []workspaceEntry{}}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == ".git" {
			return filepath.SkipDir
		}
		if relative == "." {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		item := workspaceEntry{path: relative, mode: info.Mode()}
		switch {
		case info.Mode().IsRegular():
			item.content, err = os.ReadFile(path)
		case info.Mode()&os.ModeSymlink != 0:
			item.link, err = os.Readlink(path)
		}
		if err != nil {
			return err
		}
		state.entries = append(state.entries, item)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(state.entries, func(i, j int) bool { return state.entries[i].path < state.entries[j].path })
	command := exec.Command("git", "-C", root, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored=no")
	status, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	state.gitStatus = bytes.Clone(status)
	return state
}

func assertExactWorkspaceUnchanged(t *testing.T, root string, before exactWorkspaceState) {
	t.Helper()
	after := captureExactWorkspace(t, root)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("workspace changed on a rejected objective:\nbefore=%#v\nafter=%#v", before, after)
	}
}
