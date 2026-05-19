package odn

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildProjectRunProfileUsesLLMFromWorkspaceEvidence(t *testing.T) {
	workspace := t.TempDir()
	writeProfileTestFile(t, workspace, "build.zig", "pub fn build(b: *std.Build) void {}\n")
	writeProfileTestFile(t, workspace, "src/main.zig", "pub fn main() void {}\n")
	client := &fakeCommandDecisionClient{responses: []string{
		`{"summary":"Zig project inferred from build.zig","languages":["Zig"],"frameworks":[],"run_commands":["zig build run"],"test_commands":["zig build test"],"build_commands":["zig build"],"evidence":["build.zig","src/main.zig"]}`,
	}}

	profile, err := BuildProjectRunProfile(context.Background(), workspace, client, ProjectRunProfileConfig{MaxFiles: 20})
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatalf("llm calls = %d, want 1", client.calls)
	}
	if !strings.Contains(client.prompts[0], "build.zig") || !strings.Contains(client.prompts[0], "src/main.zig") {
		t.Fatalf("profile prompt missing workspace evidence: %s", client.prompts[0])
	}
	if profile.Summary != "Zig project inferred from build.zig" {
		t.Fatalf("summary = %q", profile.Summary)
	}
	if strings.Join(profile.RunCommands, ",") != "zig build run" {
		t.Fatalf("run commands = %#v", profile.RunCommands)
	}
	if strings.Join(profile.TestCommands, ",") != "zig build test" {
		t.Fatalf("test commands = %#v", profile.TestCommands)
	}
}

func TestProjectWorkspaceSnapshotIsGenericFileEvidence(t *testing.T) {
	workspace := t.TempDir()
	writeProfileTestFile(t, workspace, "composer.json", `{"scripts":{"test":"phpunit"}}`)
	writeProfileTestFile(t, workspace, "app/Http/Controllers/HomeController.php", "<?php\n")
	if err := os.MkdirAll(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeProfileTestFile(t, workspace, ".git/config", "hidden")

	snapshot, err := BuildProjectWorkspaceSnapshot(workspace, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"composer.json", "app/", "app/Http/", "HomeController.php"} {
		if !strings.Contains(snapshot, want) {
			t.Fatalf("snapshot missing %q:\n%s", want, snapshot)
		}
	}
	if strings.Contains(snapshot, ".git/config") {
		t.Fatalf("snapshot should skip .git:\n%s", snapshot)
	}
}

func TestParseProjectRunProfileRejectsMissingSummary(t *testing.T) {
	_, err := parseProjectRunProfile(`{"languages":[],"frameworks":[],"run_commands":[],"test_commands":[],"build_commands":[],"evidence":[]}`)
	if err == nil {
		t.Fatal("expected missing summary to fail")
	}
}

func TestProjectRunProfilerPromptDoesNotHardcodeProjectTypes(t *testing.T) {
	prompt := buildProjectRunProfilerPrompt()
	for _, forbidden := range []string{"go.mod means", "composer.json means", "package.json means", "build.zig means"} {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			t.Fatalf("project profiler prompt contains hardcoded mapping %q:\n%s", forbidden, prompt)
		}
	}
}

func TestProjectRunProfileFormattingCarriesCommands(t *testing.T) {
	profile := ProjectRunProfile{
		Summary:       "Java app",
		Languages:     []string{"Java"},
		Frameworks:    []string{"Spring"},
		RunCommands:   []string{"./mvnw spring-boot:run"},
		TestCommands:  []string{"./mvnw test"},
		BuildCommands: []string{"./mvnw package"},
		Evidence:      []string{"pom.xml"},
	}
	formatted := formatProjectRunProfile(profile)
	for _, want := range []string{"Java app", "./mvnw spring-boot:run", "./mvnw test", "pom.xml"} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("formatted profile missing %q:\n%s", want, formatted)
		}
	}
}

func writeProfileTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildProjectRunProfileRequiresLLM(t *testing.T) {
	workspace := t.TempDir()
	writeProfileTestFile(t, workspace, "README.md", "demo")
	_, err := BuildProjectRunProfile(context.Background(), workspace, nil, ProjectRunProfileConfig{})
	if err == nil {
		t.Fatal("expected missing LLM to fail")
	}
}

func TestProjectRunProfileParserCleansDuplicateValues(t *testing.T) {
	profile, err := parseProjectRunProfile(`{"summary":"x","languages":["Go","Go",""],"frameworks":[],"run_commands":["go run .","go run ."],"test_commands":[],"build_commands":[],"evidence":["go.mod","go.mod"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(profile.Languages, ",") != "Go" {
		t.Fatalf("languages = %#v", profile.Languages)
	}
	if strings.Join(profile.RunCommands, ",") != "go run ." {
		t.Fatalf("run commands = %#v", profile.RunCommands)
	}
	if strings.Join(profile.Evidence, ",") != "go.mod" {
		t.Fatalf("evidence = %#v", profile.Evidence)
	}
}

func TestProjectRunProfileSnapshotRequiresWorkspace(t *testing.T) {
	_, err := BuildProjectWorkspaceSnapshot("", 10)
	if err == nil {
		t.Fatal("expected empty workspace to fail")
	}
}

func TestProjectRunProfileUsesStructuredJSONSchema(t *testing.T) {
	workspace := t.TempDir()
	writeProfileTestFile(t, workspace, "README.md", "demo")
	client := &fakeCommandDecisionClient{responses: []string{
		`{"summary":"unknown","languages":[],"frameworks":[],"run_commands":[],"test_commands":[],"build_commands":[],"evidence":["README.md"]}`,
	}}
	if _, err := BuildProjectRunProfile(context.Background(), workspace, client, ProjectRunProfileConfig{}); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 || client.requests[0].Format == nil {
		t.Fatalf("missing structured format request: %#v", client.requests)
	}
}

func TestProjectRunProfileSnapshotRespectsLimit(t *testing.T) {
	workspace := t.TempDir()
	for _, rel := range []string{"a.txt", "b.txt", "c.txt"} {
		writeProfileTestFile(t, workspace, rel, rel)
	}
	snapshot, err := BuildProjectWorkspaceSnapshot(workspace, 2)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(snapshot, "\n")
	if len(lines) > 4 {
		t.Fatalf("snapshot should include workspace line, files heading, and 2 entries:\n%s", snapshot)
	}
}

func TestProjectRunProfileOutputDoesNotWriteCommandOutput(t *testing.T) {
	workspace := t.TempDir()
	writeProfileTestFile(t, workspace, "go.mod", "module example")
	client := &fakeCommandDecisionClient{responses: []string{
		`{"summary":"Go","languages":["Go"],"frameworks":[],"run_commands":["go run ."],"test_commands":[],"build_commands":[],"evidence":["go.mod"]}`,
	}}
	stdout := &bytes.Buffer{}
	_, err := BuildProjectRunProfile(context.Background(), workspace, client, ProjectRunProfileConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}
