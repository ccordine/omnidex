package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
	toolruntime "github.com/gryph/omnidex/internal/tools"
	"github.com/gryph/omnidex/internal/workspace"
)

func TestV3WorkspaceWriteMutatesOnlyAuthoritativeScope(t *testing.T) {
	configuredRoot := t.TempDir()
	jobRoot := t.TempDir()
	for _, root := range []string{configuredRoot, jobRoot} {
		if err := os.WriteFile(filepath.Join(root, "routing.txt"), []byte("old\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	base := workspace.New(true, configuredRoot, 100, 4000)
	scanner, err := base.Scoped(jobRoot)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := withV3WorkspaceScope(context.Background(), v3WorkspaceScope{Scanner: scanner, Root: scanner.Root(), Source: "job_metadata"})
	if err != nil {
		t.Fatal(err)
	}
	registry := newV3ToolRegistry(&Service{workspace: base})
	result, err := registry.Execute(ctx, toolruntime.Call{Name: "workspace.write", Input: map[string]any{
		"path": "routing.txt", "operation": "replace", "content": "authoritative routing\n",
	}}, toolruntime.ExecuteOptions{Allowed: []string{"workspace.write"}, RequireListed: true})
	if err != nil {
		t.Fatal(err)
	}
	jobContent, err := os.ReadFile(filepath.Join(jobRoot, "routing.txt"))
	if err != nil {
		t.Fatal(err)
	}
	configuredContent, err := os.ReadFile(filepath.Join(configuredRoot, "routing.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(jobContent) != "authoritative routing\n" || string(configuredContent) != "old\n" {
		t.Fatalf("job=%q configured=%q", jobContent, configuredContent)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Kind != evidence.KindGeneratedDiff || !metadataFlag(result.Evidence[0].Metadata, "succeeded") {
		t.Fatalf("patch evidence=%#v", result.Evidence)
	}
}

func TestV3WorkspaceWriteIsTheOnlyRegisteredMutationTool(t *testing.T) {
	registry := newV3ToolRegistry(&Service{})
	if _, ok := registry.Spec("workspace.write"); !ok {
		t.Fatal("workspace.write is not registered")
	}
	if _, ok := registry.Spec("workspace.patch"); ok {
		t.Fatal("obsolete workspace.patch fallback remains registered")
	}
}

func TestV3WorkspaceWriteRequiresExplicitFileLifecycle(t *testing.T) {
	root := t.TempDir()
	base := workspace.New(true, root, 100, 4000)
	scanner, err := base.Scoped(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := withV3WorkspaceScope(context.Background(), v3WorkspaceScope{Scanner: scanner, Root: scanner.Root(), Source: "job_metadata"})
	if err != nil {
		t.Fatal(err)
	}

	create := toolruntime.Call{Name: "workspace.write", Input: map[string]any{
		"path": "task.go", "operation": "create", "content": "package task\n",
	}}
	if _, err := executeV3WorkspaceWrite(ctx, create); err != nil {
		t.Fatal(err)
	}
	if _, err := executeV3WorkspaceWrite(ctx, create); !toolruntime.IsCallRejected(err) || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate create err=%v, want explicit rejection", err)
	}

	replace := toolruntime.Call{Name: "workspace.write", Input: map[string]any{
		"path": "task.go", "operation": "replace", "content": "package task\n\nconst Ready = true\n",
	}}
	if _, err := executeV3WorkspaceWrite(ctx, replace); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "task.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "package task\n\nconst Ready = true\n" {
		t.Fatalf("replacement content=%q", content)
	}

	deleteCall := toolruntime.Call{Name: "workspace.write", Input: map[string]any{
		"path": "task.go", "operation": "delete", "content": "",
	}}
	if _, err := executeV3WorkspaceWrite(ctx, deleteCall); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "task.go")); !os.IsNotExist(err) {
		t.Fatalf("deleted file still exists: %v", err)
	}
}

func TestV3WorkspaceWriteRejectsEscapesSymlinksAndProtectedFiles(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	base := workspace.New(true, root, 100, 4000)
	scanner, err := base.Scoped(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := withV3WorkspaceScope(context.Background(), v3WorkspaceScope{Scanner: scanner, Root: scanner.Root(), Source: "job_metadata"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../outside.txt", "linked/outside.txt", ".env", ".git/config"} {
		_, err := executeV3WorkspaceWrite(ctx, toolruntime.Call{Name: "workspace.write", Input: map[string]any{
			"path": path, "operation": "create", "content": "must not be written\n",
		}})
		if !toolruntime.IsCallRejected(err) {
			t.Errorf("path %q err=%v, want recoverable security rejection", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "outside.txt")); !os.IsNotExist(err) {
		t.Fatalf("workspace.write escaped through symlink: %v", err)
	}
}

func TestV3CommandRunIsShellFreeAndRecordsObservedResult(t *testing.T) {
	root := t.TempDir()
	base := workspace.New(true, root, 100, 4000)
	scanner, err := base.Scoped(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := withV3WorkspaceScope(context.Background(), v3WorkspaceScope{Scanner: scanner, Root: scanner.Root(), Source: "job_metadata"})
	if err != nil {
		t.Fatal(err)
	}
	registry := newV3ToolRegistry(&Service{workspace: base})
	result, err := registry.Execute(ctx, toolruntime.Call{Name: "command.run", Input: map[string]any{
		"program": "go",
		"args":    []string{"version"},
	}}, toolruntime.ExecuteOptions{Allowed: []string{"command.run"}, RequireListed: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output["succeeded"] != true || !strings.Contains(result.Output["stdout"].(string), "go version") {
		t.Fatalf("command output=%#v", result.Output)
	}
	if len(result.Evidence) != 1 || !metadataFlag(result.Evidence[0].Metadata, "succeeded") {
		t.Fatalf("command evidence=%#v", result.Evidence)
	}
	_, err = registry.Execute(ctx, toolruntime.Call{Name: "command.run", Input: map[string]any{
		"program": "bash",
		"args":    []string{"-lc", "touch escaped"},
	}}, toolruntime.ExecuteOptions{Allowed: []string{"command.run"}, RequireListed: true})
	if err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("arbitrary shell err=%v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "escaped")); !os.IsNotExist(statErr) {
		t.Fatalf("rejected shell created a file: %v", statErr)
	}
}

func TestV3CommandRunAllowsBoundedGoModuleInitialization(t *testing.T) {
	root := t.TempDir()
	base := workspace.New(true, root, 100, 4000)
	scanner, err := base.Scoped(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := withV3WorkspaceScope(context.Background(), v3WorkspaceScope{Scanner: scanner, Root: scanner.Root(), Source: "job_metadata"})
	if err != nil {
		t.Fatal(err)
	}
	registry := newV3ToolRegistry(&Service{workspace: base})
	result, err := registry.Execute(ctx, toolruntime.Call{Name: "command.run", Input: map[string]any{
		"program": "go",
		"args":    []string{"mod", "init", "pockettasks"},
	}}, toolruntime.ExecuteOptions{Allowed: []string{"command.run"}, RequireListed: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output["succeeded"] != true {
		t.Fatalf("command output=%#v", result.Output)
	}
	contents, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "module pockettasks") {
		t.Fatalf("go.mod=%q", contents)
	}
}

func TestV3CommandAllowsOnlyBoundedWorkspaceInitializers(t *testing.T) {
	allowed := []struct {
		program string
		args    []string
	}{
		{program: "go", args: []string{"mod", "init", "pockettasks"}},
		{program: "cargo", args: []string{"init", "--name", "caesar_lab", "--vcs", "none", "."}},
		{program: "npm", args: []string{"init", "--yes"}},
	}
	for _, test := range allowed {
		if err := validateV3Command(test.program, test.args); err != nil {
			t.Errorf("%s %q rejected: %v", test.program, test.args, err)
		}
	}

	rejected := []struct {
		program string
		args    []string
	}{
		{program: "go", args: []string{"mod", "tidy"}},
		{program: "go", args: []string{"mod", "init"}},
		{program: "cargo", args: []string{"init", "../outside"}},
		{program: "cargo", args: []string{"init", "--vcs", "git", "."}},
		{program: "npm", args: []string{"init", "create-vite"}},
		{program: "npm", args: []string{"install", "left-pad"}},
	}
	for _, test := range rejected {
		if err := validateV3Command(test.program, test.args); err == nil {
			t.Errorf("%s %q was not rejected", test.program, test.args)
		}
	}
}

func TestV3ToolPreflightFailuresAreRecoverableObservations(t *testing.T) {
	root := t.TempDir()
	base := workspace.New(true, root, 100, 4000)
	scanner, err := base.Scoped(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := withV3WorkspaceScope(context.Background(), v3WorkspaceScope{Scanner: scanner, Root: scanner.Root(), Source: "job_metadata"})
	if err != nil {
		t.Fatal(err)
	}

	_, commandErr := executeV3Command(ctx, toolruntime.Call{Name: "command.run", Input: map[string]any{
		"program": "go",
		"args":    []string{"mod", "tidy"},
	}})
	if !toolruntime.IsCallRejected(commandErr) {
		t.Fatalf("command error=%v, want recoverable observation", commandErr)
	}
	_, writeErr := executeV3WorkspaceWrite(ctx, toolruntime.Call{Name: "workspace.write", Input: map[string]any{
		"path": "missing.txt", "operation": "replace", "content": "replacement\n",
	}})
	if !toolruntime.IsCallRejected(writeErr) {
		t.Fatalf("write error=%v, want recoverable observation", writeErr)
	}
}

func TestV3WorkspaceWriteRejectsNoOpReplacement(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module pocket_tasks\n\ngo 1.26.3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := workspace.New(true, root, 100, 4000)
	scanner, err := base.Scoped(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := withV3WorkspaceScope(context.Background(), v3WorkspaceScope{Scanner: scanner, Root: scanner.Root(), Source: "job_metadata"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = executeV3WorkspaceWrite(ctx, toolruntime.Call{Name: "workspace.write", Input: map[string]any{
		"path": "go.mod", "operation": "replace", "content": "module pocket_tasks\n\ngo 1.26.3\n",
	}})
	if !toolruntime.IsCallRejected(err) {
		t.Fatalf("no-op write error=%v, want recoverable rejection", err)
	}
	for _, required := range []string{"does not change", "go.mod"} {
		if !strings.Contains(err.Error(), required) {
			t.Fatalf("no-op rejection omitted %q: %v", required, err)
		}
	}
}

func TestV3GitDiffRequiresExternalExecutorGuards(t *testing.T) {
	if err := validateV3Command("git", []string{"diff", "--"}); err == nil || !strings.Contains(err.Error(), "--no-ext-diff") {
		t.Fatalf("unguarded git diff err=%v", err)
	}
	if err := validateV3Command("git", []string{"diff", "--no-ext-diff", "--no-textconv", "--"}); err != nil {
		t.Fatalf("guarded git diff rejected: %v", err)
	}
}

func TestV3CommandRejectsEmbeddedWorkspaceEscapes(t *testing.T) {
	for _, args := range [][]string{
		{"test", "-coverprofile=/tmp/coverage.out", "./..."},
		{"test", "-coverprofile=reports/../../../outside", "./..."},
		{"test", "--output=../../outside", "./..."},
	} {
		if err := validateV3Command("go", args); err == nil || !strings.Contains(err.Error(), "escapes") {
			t.Fatalf("escaping args=%q err=%v", args, err)
		}
	}
	if err := validateV3Command("go", []string{"test", "-coverprofile=reports/coverage.out", "./..."}); err != nil {
		t.Fatalf("workspace-relative report path rejected: %v", err)
	}
}

func TestV3CommandEnvironmentOverridesPagerWithoutDuplicates(t *testing.T) {
	env := v3CommandEnvironment([]string{"PATH=/bin", "PAGER=less", "GIT_PAGER=custom"})
	if strings.Join(env, "|") != "PATH=/bin|GIT_PAGER=cat|PAGER=cat" {
		t.Fatalf("environment=%#v", env)
	}
}

func TestV3ExecutionCapabilitiesRequireLiveTools(t *testing.T) {
	capabilities := availableV3Capabilities([]string{"workspace.write", "command.run"})
	for _, required := range []string{capabilityWorkspaceWrite, capabilityCommandExecute} {
		if !containsString(capabilities, required) {
			t.Fatalf("capabilities=%v missing %s", capabilities, required)
		}
	}
}

func TestV3ExecutionEvidenceRequiresObservedSuccess(t *testing.T) {
	failed := evidence.Record{Kind: evidence.KindGeneratedDiff, Metadata: map[string]any{"mutation": true, "succeeded": false}}
	if hasV3ExecutionEvidence([]evidence.Record{failed}) {
		t.Fatal("failed mutation must not count as execution evidence")
	}
	passed := evidence.Record{Kind: evidence.KindGeneratedDiff, Metadata: map[string]any{"mutation": true, "succeeded": true}}
	if !hasV3ExecutionEvidence([]evidence.Record{passed}) {
		t.Fatal("successful generated diff must count as execution evidence")
	}
}

func TestUnresolvedV3CommandFailuresUsesLatestSameCommandResult(t *testing.T) {
	records := []evidence.Record{
		{ID: 1, Kind: evidence.KindTestResult, Command: "go test ./...", Metadata: map[string]any{"succeeded": false}},
		{ID: 2, Kind: evidence.KindTestResult, Command: "npm test", Metadata: map[string]any{"succeeded": false}},
		{ID: 3, Kind: evidence.KindTestResult, Command: "go test ./...", Metadata: map[string]any{"succeeded": true}},
	}
	failed := unresolvedV3CommandFailures(records)
	if strings.Join(failed, ",") != "npm test" {
		t.Fatalf("unresolved failures=%v", failed)
	}
}

func TestV3FinalizationRequiresPatchAndSuccessfulCommandForWorkspaceWrite(t *testing.T) {
	intent := artifacts.IntentArtifact{
		UserGoal:             "Patch routing",
		RequiresAction:       true,
		RequiredCapabilities: []string{capabilityWorkspaceWrite, capabilityCommandExecute},
		Objectives: []artifacts.Objective{{
			ID:                   "patch",
			Description:          "Patch routing",
			Priority:             100,
			RequiresAction:       true,
			RequiredCapabilities: []string{capabilityWorkspaceWrite, capabilityCommandExecute},
			AcceptanceCriteria:   []string{"patch is verified"},
		}},
	}
	verification := artifacts.VerificationArtifact{
		Verdict:              artifacts.VerificationVerdictPass,
		IndependentChallenge: true,
		ObjectiveCoverage: []artifacts.ObjectiveCoverage{{
			ObjectiveID: "patch",
			Satisfied:   true,
			EvidenceIDs: []int64{1},
		}},
	}
	records := []evidence.Record{{ID: 1, Kind: evidence.KindGeneratedDiff, Metadata: map[string]any{"mutation": true, "succeeded": true}}}
	err := validateV3Finalization(intent, verification, records, "Routing was patched.")
	if err == nil || !strings.Contains(err.Error(), "command.execute") {
		t.Fatalf("missing verification command err=%v", err)
	}
}
