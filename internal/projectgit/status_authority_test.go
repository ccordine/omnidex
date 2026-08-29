package projectgit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type scriptedRunner struct {
	outputs map[string]string
	errors  map[string]error
}

func (runner scriptedRunner) Output(_ context.Context, _ string, args ...string) (string, error) {
	key := strings.Join(args, "\x1f")
	if err, exists := runner.errors[key]; exists {
		return "", err
	}
	if value, exists := runner.outputs[key]; exists {
		return value, nil
	}
	return "", errors.New("unscripted git command: " + strings.Join(args, " "))
}

func TestCollectStatusPropagatesEveryAuthoritativeGitCommandFailure(t *testing.T) {
	location := repositoryMarker(t)
	for _, key := range []string{
		commandKey("rev-parse", "--is-inside-work-tree"),
		commandKey("rev-parse", "--show-toplevel"),
		commandKey("branch", "--show-current"),
		commandKey("rev-parse", "--short", "HEAD"),
		commandKey("config", "--get", "branch.main.remote"),
		commandKey("config", "--get", "branch.main.merge"),
		commandKey("config", "--get", "remote.origin.url"),
		commandKey("status", "--porcelain=v1", "-u"),
		commandKey("stash", "list"),
		commandKey("log", "-12", "--format=%H%x00%s%x00%an%x00%ar%x1e"),
	} {
		t.Run(strings.ReplaceAll(key, "\x1f", "_"), func(t *testing.T) {
			runner := validScriptedRunner()
			runner.errors[key] = errors.New("forced command failure")
			delete(runner.outputs, key)
			if _, err := collectStatus(context.Background(), location, "core-local", runner); err == nil ||
				!strings.Contains(err.Error(), "forced command failure") {
				t.Fatalf("command %q did not fail loudly: %v", key, err)
			}
		})
	}
}

func TestCollectStatusRejectsMalformedCommandAuthority(t *testing.T) {
	location := repositoryMarker(t)
	cases := []struct {
		name, key, output string
	}{
		{"classification", commandKey("rev-parse", "--is-inside-work-tree"), "false"},
		{"root", commandKey("rev-parse", "--show-toplevel"), ""},
		{"head", commandKey("rev-parse", "--short", "HEAD"), ""},
		{"status row", commandKey("status", "--porcelain=v1", "-u"), "M"},
		{"log row", commandKey("log", "-12", "--format=%H%x00%s%x00%an%x00%ar%x1e"), "malformed"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			runner := validScriptedRunner()
			runner.outputs[test.key] = test.output
			if _, err := collectStatus(context.Background(), location, "core-local", runner); err == nil {
				t.Fatalf("malformed %s authority was accepted", test.name)
			}
		})
	}
}

func TestCollectStatusRejectsMalformedAncestorRepositoryMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
	if _, err := CollectStatus(context.Background(), project, "core-local"); err == nil || !strings.Contains(err.Error(), "lacks a regular HEAD") {
		t.Fatalf("malformed ancestor repository marker did not fail loudly: %v", err)
	}
}

func TestCollectStatusRejectsMalformedAndFailedUpstreamAuthority(t *testing.T) {
	location := repositoryMarker(t)
	for _, test := range []struct {
		name  string
		alter func(scriptedRunner)
	}{
		{"incomplete config", func(r scriptedRunner) {
			r.outputs[commandKey("config", "--get", "branch.main.remote")] = "origin"
			delete(r.errors, commandKey("config", "--get", "branch.main.remote"))
		}},
		{"resolution failure", func(r scriptedRunner) {
			enableUpstream(r)
			r.errors[commandKey("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")] = errors.New("forced upstream failure")
		}},
		{"count failure", func(r scriptedRunner) {
			enableUpstream(r)
			r.errors[commandKey("rev-list", "--left-right", "--count", "@{upstream}...HEAD")] = errors.New("forced count failure")
		}},
		{"malformed counts", func(r scriptedRunner) {
			enableUpstream(r)
			r.outputs[commandKey("rev-list", "--left-right", "--count", "@{upstream}...HEAD")] = "zero 1"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := validScriptedRunner()
			test.alter(runner)
			if _, err := collectStatus(context.Background(), location, "core-local", runner); err == nil {
				t.Fatalf("invalid upstream authority was accepted")
			}
		})
	}
}

func TestDecodeStatusPayloadRequiresOneClosedTypedProjection(t *testing.T) {
	commit := Commit{Hash: "0123456789ab", Subject: "Initial", Author: "Test", RelativeDate: "now"}
	status := Status{
		Location: "/srv/project", Source: "host-bridge", IsRepo: true,
		Root: "/srv/project", Branch: "main", HeadShort: "abc123",
		ChangedFiles: []ChangedFile{}, Clean: true,
		RecentCommits: []Commit{commit}, LastCommit: &commit,
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeStatusPayload(payload)
	if err != nil || decoded.HeadShort != "abc123" {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"missing":       func(value map[string]any) { delete(value, "head_short") },
		"unknown":       func(value map[string]any) { value["agent"] = "forbidden" },
		"wrong type":    func(value map[string]any) { value["staged_count"] = "0" },
		"contradiction": func(value map[string]any) { value["clean"] = false },
	} {
		t.Run(name, func(t *testing.T) {
			var invalid map[string]any
			if err := json.Unmarshal(raw, &invalid); err != nil {
				t.Fatal(err)
			}
			mutate(invalid)
			if _, err := DecodeStatusPayload(invalid); err == nil {
				t.Fatalf("invalid payload %#v was accepted", invalid)
			}
		})
	}
}

func validScriptedRunner() scriptedRunner {
	missing := func() error { return &commandError{ExitCode: 1, Cause: errors.New("not configured")} }
	return scriptedRunner{
		outputs: map[string]string{
			commandKey("rev-parse", "--is-inside-work-tree"):                "true\n",
			commandKey("rev-parse", "--show-toplevel"):                      "/srv/project\n",
			commandKey("branch", "--show-current"):                          "main\n",
			commandKey("rev-parse", "--short", "HEAD"):                      "abc123\n",
			commandKey("status", "--porcelain=v1", "-u"):                    "",
			commandKey("stash", "list"):                                     "",
			commandKey("log", "-12", "--format=%H%x00%s%x00%an%x00%ar%x1e"): "0123456789abcdef\x00Initial\x00Test\x00now\x1e",
		},
		errors: map[string]error{
			commandKey("config", "--get", "branch.main.remote"): missing(),
			commandKey("config", "--get", "branch.main.merge"):  missing(),
			commandKey("config", "--get", "remote.origin.url"):  missing(),
		},
	}
}

func enableUpstream(runner scriptedRunner) {
	for key, value := range map[string]string{
		commandKey("config", "--get", "branch.main.remote"):                            "origin",
		commandKey("config", "--get", "branch.main.merge"):                             "refs/heads/main",
		commandKey("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"): "origin/main",
		commandKey("rev-list", "--left-right", "--count", "@{upstream}...HEAD"):        "0 0",
	} {
		delete(runner.errors, key)
		runner.outputs[key] = value
	}
}

func repositoryMarker(t *testing.T) string {
	t.Helper()
	location := t.TempDir()
	gitDir := filepath.Join(location, ".git")
	if err := os.Mkdir(gitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"HEAD": "ref: refs/heads/main\n", "config": "[core]\n"} {
		if err := os.WriteFile(filepath.Join(gitDir, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return location
}

func commandKey(args ...string) string { return strings.Join(args, "\x1f") }
