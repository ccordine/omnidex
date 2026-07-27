package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStructuredCommandDecisionDoesNotUseDoneCheckToCloseWeakLegacyLedger(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"npm init -y","done":false,"answer":""}`,
		`{"command":"npm init -y","done":false,"answer":""}`,
		`{"command":"printf 'webpack stimulus tailwind recyclr done' > setup.txt","done":false,"answer":"","objective_ledger":[{"id":"install_stimulus_js","description":"Install or account for Stimulus JS","status":"satisfied","evidence":"command output"},{"id":"install_recyclr_js","description":"Install or account for Recyclr JS","status":"satisfied","evidence":"command output"},{"id":"install_tailwind_css","description":"Install or account for Tailwind CSS","status":"satisfied","evidence":"command output"},{"id":"setup_webpack","description":"Set up webpack","status":"satisfied","evidence":"command output"}]}`,
		`{"command":"test -s setup.txt && cat setup.txt","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Project initialized and dependencies accounted for."}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "initialize_npm_project", Description: "Initialize npm project", Status: "pending"},
			{ID: "install_stimulus_js", Description: "Install or account for Stimulus JS", Status: "pending"},
			{ID: "install_recyclr_js", Description: "Install or account for Recyclr JS", Status: "pending"},
			{ID: "install_tailwind_css", Description: "Install or account for Tailwind CSS", Status: "pending"},
			{ID: "setup_webpack", Description: "Set up webpack", Status: "pending"},
		},
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   false,
		Reason: "npm init output proves package.json was initialized",
		ObjectiveLedger: []StructuredObjective{
			{ID: "initialize_npm_project", Description: "Initialize npm project", Status: "satisfied", Evidence: "npm init wrote package.json"},
		},
	}, {
		Done:   false,
		Reason: "all objectives satisfied by command evidence and planner ledger update",
		ObjectiveLedger: []StructuredObjective{
			{ID: "install_stimulus_js", Description: "Install or account for Stimulus JS", Status: "satisfied", Evidence: "command output"},
			{ID: "install_recyclr_js", Description: "Install or account for Recyclr JS", Status: "satisfied", Evidence: "command output"},
			{ID: "install_tailwind_css", Description: "Install or account for Tailwind CSS", Status: "satisfied", Evidence: "command output"},
			{ID: "setup_webpack", Description: "Set up webpack", Status: "satisfied", Evidence: "command output"},
		},
	}, {
		Done:   true,
		Reason: "readback command verified setup.txt contents",
		ObjectiveLedger: []StructuredObjective{
			{ID: "install_stimulus_js", Description: "Install or account for Stimulus JS", Status: "satisfied", Evidence: "cat setup.txt"},
			{ID: "install_recyclr_js", Description: "Install or account for Recyclr JS", Status: "satisfied", Evidence: "cat setup.txt"},
			{ID: "install_tailwind_css", Description: "Install or account for Tailwind CSS", Status: "satisfied", Evidence: "cat setup.txt"},
			{ID: "setup_webpack", Description: "Set up webpack", Status: "satisfied", Evidence: "cat setup.txt"},
		},
	}, {
		Done:   true,
		Reason: "readback command verified setup.txt contents",
		ObjectiveLedger: []StructuredObjective{
			{ID: "install_stimulus_js", Description: "Install or account for Stimulus JS", Status: "satisfied", Evidence: "cat setup.txt"},
			{ID: "install_recyclr_js", Description: "Install or account for Recyclr JS", Status: "satisfied", Evidence: "cat setup.txt"},
			{ID: "install_tailwind_css", Description: "Install or account for Tailwind CSS", Status: "satisfied", Evidence: "cat setup.txt"},
			{ID: "setup_webpack", Description: "Set up webpack", Status: "satisfied", Evidence: "cat setup.txt"},
		},
	}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"make a test project here",
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: t.TempDir(),
			PromptInterpreter:       interpreter,
			CompletionChecker:       checker,
		},
	)
	if err == nil {
		t.Fatalf("weak legacy ledger updates should not satisfy typed queue evidence; result=%#v events=%#v", result, events)
	}
	if len(checker.inputs) != 0 {
		t.Fatalf("completion checker should not close weak pending work: %#v", checker.inputs)
	}
	repeatedNpmInit := 0
	skippedNpmInit := 0
	for _, obs := range result.Observations {
		if normalizeStructuredCommandForComparison(obs.Command) == "npm init -y" && obs.ExitCode == 0 {
			repeatedNpmInit++
		}
		if strings.HasPrefix(obs.Command, "SKIPPED_REPEAT_SUCCESS:") && normalizeStructuredCommandForComparison(obs.RejectedCommand) == "npm init -y" {
			skippedNpmInit++
		}
	}
	if repeatedNpmInit != 1 || skippedNpmInit != 1 {
		t.Fatalf("expected repeated npm init to execute once and skip once, executed=%d skipped=%d observations=%#v events=%#v", repeatedNpmInit, skippedNpmInit, result.Observations, events)
	}
}

func TestStructuredCommandDecisionRequiresReadbackAfterPackageMutation(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"name":"readback-test","version":"1.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"npm pkg set scripts.start='node index.js'","done":false,"answer":""}`,
		`{"command":"npm pkg get scripts.start","done":false,"answer":""}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "add_start_script", Description: "Add a start script to package.json", Status: "pending", Source: "user_explicit", Required: true},
		},
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   true,
		Reason: "npm pkg set succeeded",
		ObjectiveLedger: []StructuredObjective{
			{ID: "add_start_script", Description: "Add a start script to package.json", Status: "satisfied", Evidence: "npm pkg set exited 0"},
		},
	}, {
		Done:   true,
		Reason: "npm pkg get read back the configured start script",
		ObjectiveLedger: []StructuredObjective{
			{ID: "add_start_script", Description: "Add a start script to package.json", Status: "satisfied", Evidence: "npm pkg get scripts.start returned node index.js"},
		},
	}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"please add a start script",
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			PromptInterpreter:       interpreter,
			CompletionChecker:       checker,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Command != "npm pkg get scripts.start" {
		t.Fatalf("final command = %q, want readback command", result.Command)
	}
	if len(checker.inputs) != 1 {
		t.Fatalf("completion checker calls = %d, want final check only after readback evidence", len(checker.inputs))
	}
	if !strings.Contains(stdout.String(), `"node index.js"`) {
		t.Fatalf("readback stdout missing start script: %q", stdout.String())
	}
	if len(checker.inputs[0].Observations) == 0 || !strings.Contains(checker.inputs[0].Observations[len(checker.inputs[0].Observations)-1].Command, "npm pkg get scripts.start") {
		t.Fatalf("completion checker should run after readback evidence: %#v", checker.inputs[0].Observations)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("ledger still pending: %#v", result.ObjectiveLedger)
	}
}

func TestStructuredCommandDecisionSeedsLedgerFromSelectedRecipe(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'bundle evidence' > bundle.txt","done":false,"answer":""}`,
		`{"command":"test -s bundle.txt","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"bundle evidence"}`,
	}}
	recipe := Recipe{
		ID:               "frontend.stimulus-tailwind-recyclr",
		Description:      "Build frontend app",
		Objectives:       []RecipeObjective{{ID: "verify_build", Description: "Verify webpack bundle"}},
		AllowedCommands:  []string{"printf"},
		EvidenceRequired: []string{"bundle exists"},
	}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		RecipeIDs: []string{recipe.ID},
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done: false,
		ObjectiveLedger: []StructuredObjective{
			{ID: "verify_build", Description: "Verify webpack bundle", Status: "pending"},
		},
	}, {
		Done:   true,
		Reason: "bundle evidence satisfies recipe objective",
		ObjectiveLedger: []StructuredObjective{
			{ID: "verify_build", Description: "Verify webpack bundle", Status: "satisfied", Evidence: "test -s bundle.txt"},
		},
	}}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"build frontend app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: t.TempDir(),
			Recipes:                 []Recipe{recipe},
			PromptInterpreter:       interpreter,
			CompletionChecker:       checker,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("recipe objective still pending: %#v", result.ObjectiveLedger)
	}
	if !structuredEventsContain(events, "recipe_selected") {
		t.Fatalf("missing recipe_selected event: %#v", events)
	}
}

func TestStructuredCommandDecisionAcceptsSelectedRecipeCompletionProbes(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recipe := Recipe{
		ID:               "probe.recipe",
		Description:      "Probe recipe",
		Objectives:       []RecipeObjective{{ID: "package_json", Description: "package.json exists"}},
		AllowedCommands:  []string{"test"},
		EvidenceRequired: []string{"package.json exists"},
		CompletionChecks: []string{"test -f package.json"},
	}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		RecipeIDs: []string{recipe.ID},
	}}}
	summarizer := &fakeContextSummarizer{contexts: []MinimalContext{{
		Summary: "unused",
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done: true,
	}}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"structured recipe probe task",
		nil,
		nil,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			Recipes:                 []Recipe{recipe},
			PromptInterpreter:       interpreter,
			ContextSummarizer:       summarizer,
			CompletionChecker:       checker,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Command != "RECIPE_COMPLETION_PROBES" {
		t.Fatalf("command = %q", result.Command)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("recipe objective still pending: %#v", result.ObjectiveLedger)
	}
	if !structuredEventsContain(events, "completion_check_accepted_from_recipe_probes") {
		t.Fatalf("missing recipe probe completion event: %#v", events)
	}
	if !structuredEventsContain(events, "adaptive_roles_collapsed") {
		t.Fatalf("missing adaptive role collapse event: %#v", events)
	}
	if len(summarizer.inputs) != 0 {
		t.Fatalf("context summarizer should be skipped after deterministic probes pass, calls=%d", len(summarizer.inputs))
	}
	if len(checker.inputs) != 0 {
		t.Fatalf("completion checker should be skipped after deterministic probes pass, calls=%d", len(checker.inputs))
	}
}

func TestStructuredPayloadCommandReusesCommandCacheForUnchangedInputs(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "marker.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(workspace, ".cache")
	first := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(context.Background(), 1, "test -f marker.txt", workspace, true, cacheRoot, &bytes.Buffer{}, &bytes.Buffer{}, nil, &first); err != nil {
		t.Fatal(err)
	}
	second := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	if err := runStructuredPayloadCommand(
		context.Background(),
		2,
		"test -f marker.txt",
		workspace,
		true,
		cacheRoot,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&second,
	); err != nil {
		t.Fatal(err)
	}
	if len(second.Observations) != 1 || !second.Observations[0].Cached {
		t.Fatalf("expected cached observation: %#v", second.Observations)
	}
	if !structuredEventsContain(events, "command_cache_hit") {
		t.Fatalf("missing command_cache_hit event: %#v", events)
	}
}

func TestStructuredPayloadCommandTimelineIncludesCommandAndOutput(t *testing.T) {
	workspace := t.TempDir()
	events := []StructuredCommandEvent{}
	result := CommandDecisionResult{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err := runStructuredPayloadCommand(
		context.Background(),
		1,
		"printf 'timeline stdout\\n'",
		workspace,
		false,
		"",
		stdout,
		stderr,
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&result,
	)
	if err != nil {
		t.Fatal(err)
	}
	started := structuredEventOfTypeForTest(events, "structured_command_started")
	if started == nil || started.Details["command"] == "" || started.Details["cwd"] != workspace {
		t.Fatalf("started event missing command/cwd: %#v", events)
	}
	finished := structuredEventOfTypeForTest(events, "structured_command_finished")
	if finished == nil {
		t.Fatalf("missing finished event: %#v", events)
	}
	if finished.Details["command"] == "" || finished.Details["cwd"] != workspace || finished.Details["exit_code"] != "0" {
		t.Fatalf("finished event missing command metadata: %#v", finished)
	}
	if !strings.Contains(finished.Details["stdout"], "timeline stdout") {
		t.Fatalf("finished event missing stdout: %#v", finished)
	}
	if finished.Details["stderr"] != "(empty)" {
		t.Fatalf("finished event should mark empty stderr: %#v", finished)
	}
}

func TestStructuredPayloadCommandCacheTimelineIncludesCachedOutput(t *testing.T) {
	workspace := t.TempDir()
	if _, stderr, err := runShellCommand(context.Background(), workspace, "git init && printf 'cached\\n' > marker.txt"); err != nil {
		t.Fatalf("setup git repo: %v stderr=%s", err, stderr)
	}
	cacheRoot := filepath.Join(workspace, ".cache")
	result := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(context.Background(), 1, "git status --short", workspace, true, cacheRoot, &bytes.Buffer{}, &bytes.Buffer{}, nil, &result); err != nil {
		t.Fatal(err)
	}

	events := []StructuredCommandEvent{}
	cached := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(
		context.Background(),
		2,
		"git status --short",
		workspace,
		true,
		cacheRoot,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&cached,
	); err != nil {
		t.Fatal(err)
	}
	hit := structuredEventOfTypeForTest(events, "command_cache_hit")
	if hit == nil {
		t.Fatalf("missing command_cache_hit: %#v", events)
	}
	if hit.Details["cached"] != "true" || hit.Details["command"] == "" || hit.Details["cwd"] != workspace {
		t.Fatalf("cache hit event missing metadata: %#v", hit)
	}
	if !strings.Contains(hit.Details["stdout"], "marker.txt") {
		t.Fatalf("cache hit missing stdout: %#v", hit)
	}
	if hit.Details["stderr"] != "(empty)" {
		t.Fatalf("cache hit should mark empty stderr: %#v", hit)
	}
}

func TestStructuredPayloadCommandCacheInvalidatesWhenInputsChange(t *testing.T) {
	workspace := t.TempDir()
	marker := filepath.Join(workspace, "marker.txt")
	if err := os.WriteFile(marker, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(workspace, ".cache")
	first := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(context.Background(), 1, "test -f marker.txt", workspace, true, cacheRoot, &bytes.Buffer{}, &bytes.Buffer{}, nil, &first); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	if err := runStructuredPayloadCommand(
		context.Background(),
		2,
		"test -f marker.txt",
		workspace,
		true,
		cacheRoot,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&second,
	); err != nil {
		t.Fatal(err)
	}
	if len(second.Observations) != 1 || second.Observations[0].Cached {
		t.Fatalf("expected fresh observation after input change: %#v", second.Observations)
	}
	if structuredEventsContain(events, "command_cache_hit") {
		t.Fatalf("unexpected command_cache_hit event after input change: %#v", events)
	}
}

func TestStructuredPayloadCommandDoesNotCacheFailures(t *testing.T) {
	workspace := t.TempDir()
	cacheRoot := filepath.Join(workspace, ".cache")
	first := CommandDecisionResult{}
	_ = runStructuredPayloadCommand(context.Background(), 1, "test -f missing.txt", workspace, true, cacheRoot, &bytes.Buffer{}, &bytes.Buffer{}, nil, &first)
	if first.ExitCode == 0 {
		t.Fatal("expected missing file command to have nonzero exit code")
	}
	if err := os.WriteFile(filepath.Join(workspace, "missing.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	if err := runStructuredPayloadCommand(
		context.Background(),
		2,
		"test -f missing.txt",
		workspace,
		true,
		cacheRoot,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&second,
	); err != nil {
		t.Fatal(err)
	}
	if len(second.Observations) != 1 || second.Observations[0].Cached {
		t.Fatalf("expected successful fresh observation, not cached failure: %#v", second.Observations)
	}
	if structuredEventsContain(events, "command_cache_hit") {
		t.Fatalf("unexpected command_cache_hit for prior failure: %#v", events)
	}
}

func TestStructuredCommandDecisionAppliesPatchToolArtifact(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "hello.txt")
	if err := os.WriteFile(target, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := `diff --git a/hello.txt b/hello.txt
--- a/hello.txt
+++ b/hello.txt
@@ -1,2 +1,2 @@
 one
-two
+TWO
`
	response, err := json.Marshal(StructuredCommandPayload{
		Command: "",
		Done:    false,
		Answer:  "",
		Tool:    "patch.apply",
		Patch:   patch,
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{
		string(response),
		`{"command":"test -f hello.txt","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"updated hello.txt"}`,
	}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"update the file",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace},
	)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "one\nTWO\n" {
		t.Fatalf("patched file = %q", string(data))
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
	if !structuredEventsContain(events, "structured_patch_apply_finished") {
		t.Fatalf("missing patch apply event: %#v", events)
	}
}

func TestStructuredCommandDecisionRejectsVagueWTTRAndRetries(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"curl -s wttr.in","done":false,"answer":""}`,
		`{"command":"printf 'specific weather evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"specific weather evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "What is the weather in Virginia right now?", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want rejected wttr + specific command", result.Observations)
	}
	if !strings.Contains(result.Observations[0].Stderr, "wttr.in command must include an explicit location path") {
		t.Fatalf("first observation should reject vague wttr: %#v", result.Observations[0])
	}
	if result.Command != "printf 'specific weather evidence\n'" {
		t.Fatalf("command = %q", result.Command)
	}
}

func TestStructuredCommandDecisionAllowsRepeatedFailedCommandRetry(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"sh -c 'exit 7'","done":false,"answer":""}`,
		`{"command":"sh -c 'exit 7'","done":false,"answer":""}`,
		`{"command":"printf 'fallback evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"fallback evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "find evidence after a failed command", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 3 {
		t.Fatalf("observations = %#v, want failed command + repeated failed command + fallback command", result.Observations)
	}
	if result.Observations[1].Command != "sh -c 'exit 7'" || result.Observations[1].ExitCode != 7 {
		t.Fatalf("second observation should execute repeated failed command under permissive retry policy: %#v", result.Observations[1])
	}
	if result.Command != "printf 'fallback evidence\n'" {
		t.Fatalf("command = %q", result.Command)
	}
	if result.Answer != "fallback evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionExhaustsRepeatedDoneWithNonzeroFailure(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":true,"answer":"done without evidence"}`,
		`{"command":"","done":true,"answer":"done without evidence"}`,
		`{"command":"","done":true,"answer":"done without evidence"}`,
		`{"command":"","done":true,"answer":"done without evidence"}`,
		`{"command":"","done":true,"answer":"done without evidence"}`,
		`{"command":"","done":true,"answer":"done without evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "create a requested filesystem state", client, stdout, stderr)
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	if _, ok := err.(CommandDecisionExhaustedError); !ok {
		t.Fatalf("err = %T %v, want CommandDecisionExhaustedError", err, err)
	}
	if result.ExitCode == 0 {
		t.Fatalf("exhausted result exit code = 0, want nonzero")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("unexpected command output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestStructuredCommandDecisionBlocksRepeatedPrematureDoneWithPendingObjectives(t *testing.T) {
	workspace := t.TempDir()
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"pwd","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"done"}`,
		`{"command":"","done":true,"answer":"done"}`,
		`{"command":"","done":true,"answer":"done"}`,
		`{"command":"printf 'should not run\n'","done":false,"answer":""}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "design_calculator_ui", Description: "Design calculator UI", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			{ID: "implement_calculator_logic", Description: "Implement calculator logic", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			{ID: "verify_calculator_app", Description: "Verify calculator app", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		},
	}}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "continue making this calculator app", nil, client, &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		PromptInterpreter:       interpreter,
	})
	if err == nil {
		t.Fatal("expected repeated premature done to stop the loop")
	}
	if _, ok := err.(CommandDecisionExhaustedError); !ok {
		t.Fatalf("err = %T %v, want CommandDecisionExhaustedError", err, err)
	}
	if client.calls != 4 {
		t.Fatalf("planner calls = %d, want stop before fifth response", client.calls)
	}
	if !result.PartialProgress {
		t.Fatal("expected partial progress after initial successful command")
	}
	if !structuredEventsContain(events, "structured_done_loop_blocked") {
		t.Fatalf("missing structured_done_loop_blocked event: %#v", events)
	}
	if got := latestStructuredFailureSummary(result.Observations); !strings.Contains(got, "anti_loop: planner returned done=true") {
		t.Fatalf("latest blocker = %q", got)
	}
}

func TestStructuredCommandDecisionRejectsEmptyCommandAndContinues(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":""}`,
		`{"command":"printf 'searched evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"searched evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "you have access to the internet and can search", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Fatalf("llm calls = %d, want 3", client.calls)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want rejection + command", result.Observations)
	}
	if result.Observations[0].Command != "" || !strings.Contains(result.Observations[0].Stderr, "empty command") {
		t.Fatalf("first observation should reject empty command: %#v", result.Observations[0])
	}
	if result.Command != "printf 'searched evidence\n'" {
		t.Fatalf("command = %q", result.Command)
	}
	if stdout.String() != "searched evidence\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
