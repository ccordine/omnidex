package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseLocalShellIntent(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		action string
		source string
		target string
		ok     bool
	}{
		{
			name:   "create test file",
			input:  "please make me a test file quickly",
			action: "create_file",
			target: "test",
			ok:     true,
		},
		{
			name:   "create named html file with article",
			input:  "make an index.html file",
			action: "create_file",
			target: "index.html",
			ok:     true,
		},
		{
			name:   "create html file in current directory context",
			input:  "in this directory, make a test index.html",
			action: "create_file",
			target: "index.html",
			ok:     true,
		},
		{
			name:   "create html file in current directory with contraction phrasing",
			input:  "okay, in this current directory, let's make a test index.html",
			action: "create_file",
			target: "index.html",
			ok:     true,
		},
		{
			name:   "create typed html file from natural phrasing",
			input:  "in this directory, make a demo html file",
			action: "create_file",
			target: "demo.html",
			ok:     true,
		},
		{
			name:   "create nested file via called and named",
			input:  "If not, create a new file called `test` and name it `index.html`.",
			action: "create_file",
			target: filepath.Join("test", "index.html"),
			ok:     true,
		},
		{
			name:   "rename file",
			input:  "rename that test file to test-2",
			action: "rename_file",
			source: "test",
			target: "test-2",
			ok:     true,
		},
		{
			name:   "run explicit command",
			input:  "run `pwd`",
			action: "run_command",
			target: "pwd",
			ok:     true,
		},
		{
			name:   "run bare command",
			input:  "ls -1",
			action: "run_command",
			target: "ls -1",
			ok:     true,
		},
		{
			name:   "detect ip query",
			input:  "what is my ip address?",
			action: "show_ip",
			ok:     true,
		},
		{
			name:   "detect open ports query",
			input:  "what ports are open?",
			action: "show_open_ports",
			ok:     true,
		},
		{
			name:   "detect detailed open ports query",
			input:  "what ports are open with process names",
			action: "show_open_ports_detailed",
			ok:     true,
		},
		{
			name:   "detect time query",
			input:  "what time is it?",
			action: "run_command",
			target: "date",
			ok:     true,
		},
		{
			name:   "detect system info query",
			input:  "show me system info",
			action: "show_system_summary",
			ok:     true,
		},
		{
			name:   "detect running processes query",
			input:  "what is currently running on this machine?",
			action: "show_running_processes",
			ok:     true,
		},
		{
			name:   "detect my name query",
			input:  "What is my name?",
			action: "run_command",
			target: "id -un",
			ok:     true,
		},
		{
			name:   "detect working directory query",
			input:  "WHat directory are we in?",
			action: "run_command",
			target: "pwd",
			ok:     true,
		},
		{
			name:   "detect working directory query via semantic fallback",
			input:  "Can you show our current workspace path?",
			action: "run_command",
			target: "pwd",
			ok:     true,
		},
		{
			name:   "detect location query",
			input:  "determine my location based on my connection",
			action: "show_network_location",
			ok:     true,
		},
		{
			name:   "detect vpn query",
			input:  "am i on vpn right now?",
			action: "show_vpn_status",
			ok:     true,
		},
		{
			name:   "detect network catalog query",
			input:  "show network tools catalog",
			action: "show_network_tools_catalog",
			ok:     true,
		},
		{
			name:   "detect install network tools query",
			input:  "install network tools",
			action: "install_network_tools",
			ok:     true,
		},
		{
			name:   "detect repo walkthrough query",
			input:  "walk me through all the current changes in this repo",
			action: "show_repo_walkthrough",
			ok:     true,
		},
		{
			name:   "detect vpn query via semantic fallback",
			input:  "Do I have any active VPN tunnels?",
			action: "show_vpn_status",
			ok:     true,
		},
		{
			name:   "detect open ports query via semantic fallback",
			input:  "List listening sockets with process details",
			action: "show_open_ports_detailed",
			ok:     true,
		},
		{
			name:  "not a shell intent",
			input: "tell me about star trek",
			ok:    false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			intent, ok := parseLocalShellIntent(tc.input, nil)
			if ok != tc.ok {
				t.Fatalf("ok=%t want=%t", ok, tc.ok)
			}
			if !tc.ok {
				return
			}
			if intent.Action != tc.action {
				t.Fatalf("action=%q want=%q", intent.Action, tc.action)
			}
			if tc.source != "" && intent.Source != tc.source {
				t.Fatalf("source=%q want=%q", intent.Source, tc.source)
			}
			if tc.target != "" {
				if intent.Target != tc.target && intent.Command != tc.target {
					t.Fatalf("target/command=%q/%q want=%q", intent.Target, intent.Command, tc.target)
				}
			}
		})
	}
}

func TestDoItIntentUsesRecentSuggestedCommand(t *testing.T) {
	state := &localShellState{
		LastSuggestedCommand: "touch test",
		LastSuggestedAt:      time.Now(),
	}
	intent, ok := parseLocalShellIntent("do it for me", state)
	if !ok {
		t.Fatal("expected do-it intent to be recognized")
	}
	if intent.Action != "run_command" {
		t.Fatalf("action=%q", intent.Action)
	}
	if intent.Command != "touch test" {
		t.Fatalf("command=%q", intent.Command)
	}
}

func TestExtractSuggestedSafeCommand(t *testing.T) {
	command, ok := extractSuggestedSafeCommand("Quick default: run `touch test` in `/tmp`")
	if !ok {
		t.Fatal("expected command extraction")
	}
	if strings.TrimSpace(command) != "touch test" {
		t.Fatalf("command=%q", command)
	}
}

func TestValidateRelativePath(t *testing.T) {
	if err := validateRelativePath("../outside"); err == nil {
		t.Fatal("expected outside path to fail")
	}
	if err := validateRelativePath("/tmp/absolute"); err == nil {
		t.Fatal("expected absolute path to fail")
	}
	if err := validateRelativePath("notes/test.txt"); err != nil {
		t.Fatalf("expected relative path to pass, got: %v", err)
	}
}

func TestParseRepositoryWorkflowIntent(t *testing.T) {
	originalCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalCWD)
	}()

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatalf("write docker-compose.yml: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "scripts", "setup-host-deps.sh"), []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatalf("write setup script: %v", err)
	}

	intent, ok := parseLocalShellIntent("check requirements and install dependencies", nil)
	if !ok || intent.Action != "run_command" || !strings.Contains(intent.Command, "setup-host-deps.sh") {
		t.Fatalf("unexpected requirement intent: ok=%t action=%s cmd=%s", ok, intent.Action, intent.Command)
	}

	intent, ok = parseLocalShellIntent("please run tests now", nil)
	if !ok || intent.Action != "run_command" || intent.Command != "go test ./..." {
		t.Fatalf("unexpected test intent: ok=%t action=%s cmd=%s", ok, intent.Action, intent.Command)
	}

	intent, ok = parseLocalShellIntent("spin up docker test environment", nil)
	if !ok || intent.Action != "run_command" || intent.Command != "docker compose up --build -d" {
		t.Fatalf("unexpected docker intent: ok=%t action=%s cmd=%s", ok, intent.Action, intent.Command)
	}
}

func TestRunLocalSafeCommandRejectsSudoFlags(t *testing.T) {
	if _, err := runLocalSafeCommand("sudo -n ls"); err == nil {
		t.Fatal("expected sudo flags to be rejected")
	}
}

func TestShouldRetryWithSudo(t *testing.T) {
	if !shouldRetryWithSudo([]string{"ss", "-lntup"}, errors.New("Permission denied")) {
		t.Fatal("expected sudo retry for permission denied")
	}
	if shouldRetryWithSudo([]string{"ss", "-lntup"}, errors.New("command not found")) {
		t.Fatal("did not expect sudo retry for missing command")
	}
	if shouldRetryWithSudo([]string{"sudo", "ss", "-lntup"}, errors.New("permission denied")) {
		t.Fatal("did not expect sudo retry when command is already sudo")
	}
}

func TestBuildSudoRetryReason(t *testing.T) {
	reason := buildSudoRetryReason([]string{"netstat", "-lntup"}, errors.New("operation not permitted"))
	if !strings.Contains(reason, "retrying `netstat -lntup` with sudo") {
		t.Fatalf("unexpected reason text: %q", reason)
	}
	if !strings.Contains(reason, "operation not permitted") {
		t.Fatalf("expected original error context in reason: %q", reason)
	}
}

func TestInferNetworkToolsInstallCommand(t *testing.T) {
	originalCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalCWD)
	}()

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	if got := inferNetworkToolsInstallCommand(); got != "" {
		t.Fatalf("expected empty command without setup script, got: %q", got)
	}

	if err := os.MkdirAll(filepath.Join(tmp, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "scripts", "setup-host-deps.sh"), []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatalf("write setup script: %v", err)
	}

	if got := inferNetworkToolsInstallCommand(); !strings.Contains(got, "setup-host-deps.sh") {
		t.Fatalf("expected setup command, got: %q", got)
	}
}

func TestCreateLocalFileCreatesParentDirectories(t *testing.T) {
	originalCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalCWD)
	}()

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	target := filepath.Join("test", "index.html")
	out, err := createLocalFile(target)
	if err != nil {
		t.Fatalf("createLocalFile: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected target file to exist: %v", err)
	}
	if !strings.Contains(out, "Executed: mkdir -p "+filepath.Dir(target)) {
		t.Fatalf("expected mkdir command in output, got: %q", out)
	}
	if !strings.Contains(out, "Executed: touch "+target) {
		t.Fatalf("expected touch command in output, got: %q", out)
	}
	if !strings.Contains(out, "Executed: ls -l "+target) {
		t.Fatalf("expected ls verification command in output, got: %q", out)
	}
}

func TestCreateLocalFileReportsExistingFile(t *testing.T) {
	originalCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalCWD)
	}()

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	target := "test"
	if err := os.WriteFile(target, []byte("already here"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out, err := createLocalFile(target)
	if err != nil {
		t.Fatalf("createLocalFile: %v", err)
	}
	if !strings.Contains(out, "File already exists:") {
		t.Fatalf("expected existing-file message, got: %q", out)
	}
}

func TestLocateNearbyRepositoryFindsSiblingRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	originalCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalCWD)
	}()

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	repo := filepath.Join(root, "old-project")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("Chdir workspace: %v", err)
	}

	got, err := locateNearbyRepository()
	if err != nil {
		t.Fatalf("locateNearbyRepository: %v", err)
	}
	wantRoot, _ := filepath.Abs(repo)
	if got.Root != wantRoot {
		t.Fatalf("root=%q want=%q", got.Root, wantRoot)
	}
	if got.Reason != "nearby repository discovered" {
		t.Fatalf("reason=%q", got.Reason)
	}
}

func TestShowRepositoryWalkthroughChronologicalOrdering(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	originalCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalCWD)
	}()

	repo := t.TempDir()
	if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	olderPath := filepath.Join(repo, "older.txt")
	newerPath := filepath.Join(repo, "newer.txt")
	if err := os.WriteFile(olderPath, []byte("older\n"), 0o644); err != nil {
		t.Fatalf("write older: %v", err)
	}
	if err := os.WriteFile(newerPath, []byte("newer\n"), 0o644); err != nil {
		t.Fatalf("write newer: %v", err)
	}

	olderTime := time.Now().Add(-2 * time.Hour)
	newerTime := time.Now().Add(-1 * time.Minute)
	if err := os.Chtimes(olderPath, olderTime, olderTime); err != nil {
		t.Fatalf("chtimes older: %v", err)
	}
	if err := os.Chtimes(newerPath, newerTime, newerTime); err != nil {
		t.Fatalf("chtimes newer: %v", err)
	}

	if err := os.Chdir(repo); err != nil {
		t.Fatalf("Chdir repo: %v", err)
	}
	out, err := showRepositoryWalkthrough()
	if err != nil {
		t.Fatalf("showRepositoryWalkthrough: %v", err)
	}
	if !strings.Contains(out, "Repository walkthrough:") {
		t.Fatalf("missing walkthrough header:\n%s", out)
	}
	if !strings.Contains(out, "chronological_changes (most recent first):") {
		t.Fatalf("missing chronological section:\n%s", out)
	}

	newerIdx := strings.Index(out, "newer.txt")
	olderIdx := strings.Index(out, "older.txt")
	if newerIdx < 0 || olderIdx < 0 {
		t.Fatalf("expected both files in output:\n%s", out)
	}
	if newerIdx > olderIdx {
		t.Fatalf("expected newer.txt before older.txt:\n%s", out)
	}
}
