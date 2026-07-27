package worker

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func TestIsLowSignalChatInstruction(t *testing.T) {
	tests := []struct {
		name        string
		instruction string
		pipeline    string
		want        bool
	}{
		{name: "simple checkin", instruction: "test", pipeline: model.PipelineChat, want: true},
		{name: "greeting phrase", instruction: "hello there", pipeline: model.PipelineChat, want: true},
		{name: "ping punctuation", instruction: "ping?", pipeline: model.PipelineChat, want: true},
		{name: "non chat pipeline", instruction: "test", pipeline: model.PipelineAssistant, want: false},
		{name: "concrete request", instruction: "write a migration for users table", pipeline: model.PipelineChat, want: false},
		{name: "code-like token", instruction: "docker compose", pipeline: model.PipelineChat, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isLowSignalChatInstruction(tc.instruction, tc.pipeline)
			if got != tc.want {
				t.Fatalf("isLowSignalChatInstruction(%q, %q)=%v, want %v", tc.instruction, tc.pipeline, got, tc.want)
			}
		})
	}
}

func TestBuildSuccessfulJobPlaybookCapturesReusableSteps(t *testing.T) {
	details := model.JobDetails{
		Job: model.Job{
			ID:          42,
			Instruction: "Build a React note app with CRUD and in-memory storage",
			Pipeline:    model.PipelineAssistant,
			Status:      model.JobStatusCompleted,
			Result:      "React note app completed and verified.",
		},
		Steps: []model.Step{
			{ID: 1, Action: "v3_planning", Status: model.StepStatusCompleted, Output: "Plan source files and verification."},
			{ID: 2, Action: "v3_subtask", Status: model.StepStatusCompleted, Output: "Wrote src/App.jsx with note CRUD state."},
			{ID: 3, Action: "v3_verification", Status: model.StepStatusCompleted, Output: "npm test passed."},
			{ID: 4, Action: "v3_memory_review", Status: model.StepStatusPending, Output: "not done"},
		},
		Contexts: []model.StepContext{
			{ID: 1, StepID: 2, Key: "tooling", Value: "cat > src/App.jsx wrote component content"},
			{ID: 2, StepID: 3, Key: "verification", Value: "npm test exited 0"},
		},
	}

	got := buildSuccessfulJobPlaybook(details)
	for _, want := range []string{
		"Successful execution playbook",
		"Build a React note app",
		"v3_subtask: Wrote src/App.jsx",
		"tooling: cat > src/App.jsx",
		"v3_verification: npm test passed",
		"Reuse guidance:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("playbook missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "not done") {
		t.Fatalf("pending step leaked into playbook:\n%s", got)
	}
}

func TestBuildSuccessfulJobPlaybookOmitsRetrievalDump(t *testing.T) {
	dump := "Research chunk metadata:\nsource_url=https://vite.dev/config/\nResearch memory topic=https://vite.dev/config/ content: very long docs"
	details := model.JobDetails{
		Job: model.Job{
			ID:          43,
			Instruction: "Answer a smoke request",
			Pipeline:    model.PipelineAssistant,
			Status:      model.JobStatusCompleted,
			Result:      dump,
		},
		Steps: []model.Step{
			{ID: 1, Action: "v3_response_draft", Status: model.StepStatusCompleted, Output: dump},
			{ID: 2, Action: "v3_verification", Status: model.StepStatusCompleted, Output: "verdict=pass supported_claims=1 unsupported_claims=0"},
		},
	}

	got := buildSuccessfulJobPlaybook(details)
	if strings.Contains(got, "Research chunk metadata") || strings.Contains(got, "Research memory topic=") {
		t.Fatalf("retrieval dump leaked into playbook:\n%s", got)
	}
	if !strings.Contains(got, "noisy retrieval context omitted") {
		t.Fatalf("missing compact retrieval note:\n%s", got)
	}
}

func TestSuccessfulJobPlaybookTagsIncludeTopicsAndTrust(t *testing.T) {
	tags := successfulJobPlaybookTags(model.Job{
		Instruction: "Build a React Vite note app",
		Pipeline:    model.PipelineAssistant,
		Metadata:    json.RawMessage(`{"session_id":"chat-1"}`),
	}, map[string]string{"tags": "frontend,react"})

	for _, want := range []string{"frontend", "react", "procedural", "trust:approved", "success-playbook", "learned-skill", "pipeline:assistant", "topic:vite", "topic:note"} {
		found := false
		for _, got := range tags {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("tags missing %q: %#v", want, tags)
		}
	}
}

func TestPromptBlockFormatting(t *testing.T) {
	got := promptBlock("Retrieved Memory", " line one \nline two ")
	want := "<RETRIEVED_MEMORY>\nline one \nline two\n</RETRIEVED_MEMORY>"
	if got != want {
		t.Fatalf("promptBlock mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestPromptBlockEscapesTagLikeContent(t *testing.T) {
	got := promptBlock("User Instruction", "run <script>alert(1)</script>")
	if !strings.Contains(got, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("expected prompt block body to escape angle brackets, got: %q", got)
	}
	if strings.Contains(got, "<script>") {
		t.Fatalf("expected raw tag-like content to be escaped, got: %q", got)
	}
}

func TestResolveAutonomyMode(t *testing.T) {
	tests := []struct {
		name     string
		job      model.Job
		expected string
	}{
		{name: "chat default on", job: model.Job{Pipeline: model.PipelineChat}, expected: "on"},
		{name: "assistant default off", job: model.Job{Pipeline: model.PipelineAssistant}, expected: "off"},
		{name: "explicit off", job: model.Job{Pipeline: model.PipelineChat, Metadata: json.RawMessage(`{"autonomy_mode":"off"}`)}, expected: "off"},
		{name: "explicit on", job: model.Job{Pipeline: model.PipelineAssistant, Metadata: json.RawMessage(`{"autonomy_mode":"on"}`)}, expected: "on"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveAutonomyMode(tc.job)
			if got != tc.expected {
				t.Fatalf("resolveAutonomyMode()=%q, want %q", got, tc.expected)
			}
		})
	}
}

func TestMustAskForClarification(t *testing.T) {
	if !mustAskForClarification("Should I drop production database?", "drop table users") {
		t.Fatalf("expected safety-critical clarification to require user input")
	}
	if mustAskForClarification("What file name should I use?", "create a test document") {
		t.Fatalf("expected non-safety clarification to be skippable in autonomy mode")
	}
}

func TestIsSimpleFileTaskInstruction(t *testing.T) {
	if !isSimpleFileTaskInstruction("Could you create a test document quickly", model.PipelineChat) {
		t.Fatalf("expected simple file task to be detected")
	}
	if !isSimpleFileTaskInstruction("in this directory, make an index.html", model.PipelineChat) {
		t.Fatalf("expected filename-based simple file task to be detected")
	}
	if isSimpleFileTaskInstruction("Create a test document in docker container", model.PipelineChat) {
		t.Fatalf("expected complex docker request to be excluded")
	}
	if isSimpleFileTaskInstruction("Create a test document quickly", model.PipelineAssistant) {
		t.Fatalf("expected non-chat pipeline to be excluded")
	}
}

func TestShouldForceCodeOnlyResponse(t *testing.T) {
	t.Run("explicit preference in instruction", func(t *testing.T) {
		job := model.Job{
			Pipeline:    model.PipelineChat,
			Instruction: "Return only code with no markdown and no backticks.",
		}
		if !shouldForceCodeOnlyResponse(job, map[string]string{}, "llama3.2") {
			t.Fatalf("expected explicit code-only instruction to force code-only response")
		}
	})

	t.Run("coder model with code generation request", func(t *testing.T) {
		job := model.Job{
			Pipeline:    model.PipelineChat,
			Instruction: "Create an index.html file with a starter page.",
		}
		if !shouldForceCodeOnlyResponse(job, map[string]string{}, "qwen3-coder-next") {
			t.Fatalf("expected coder model with code generation request to force code-only response")
		}
	})

	t.Run("coder model with non-code request", func(t *testing.T) {
		job := model.Job{
			Pipeline:    model.PipelineChat,
			Instruction: "What's the weather today in Austin?",
		}
		if shouldForceCodeOnlyResponse(job, map[string]string{}, "qwen3-coder-next") {
			t.Fatalf("did not expect non-code request to force code-only response")
		}
	})
}

func TestNormalizeCodeOnlyResponse(t *testing.T) {
	input := strings.Join([]string{
		"Here is the file:",
		"```html",
		"<!doctype html>",
		"<html><body>Hello</body></html>",
		"```",
		"",
		"Sources:",
		"- user_instruction",
	}, "\n")

	got := normalizeCodeOnlyResponse(input)
	want := strings.Join([]string{
		"<!doctype html>",
		"<html><body>Hello</body></html>",
	}, "\n")
	if got != want {
		t.Fatalf("normalizeCodeOnlyResponse mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestIsDeterministicLocalActionReviewInstruction(t *testing.T) {
	if !isDeterministicLocalActionReviewInstruction("Deterministic post-action review step (required):\n- compare evidence") {
		t.Fatalf("expected deterministic local action review marker to be detected")
	}
	if isDeterministicLocalActionReviewInstruction("create index.html in current directory") {
		t.Fatalf("did not expect normal instruction to be flagged as deterministic local action review")
	}
}

func TestShouldIncludeFileDefaultHint(t *testing.T) {
	if !shouldIncludeFileDefaultHint("create a file with a default name", "what filename should I use?") {
		t.Fatalf("expected file-related request to include file default hint")
	}
	if shouldIncludeFileDefaultHint("weather in Fredericksburg, Virginia", "should I use web search?") {
		t.Fatalf("did not expect non-file request to include file default hint")
	}
}

func TestBuildAutonomousRewritePrompt(t *testing.T) {
	job := model.Job{
		Pipeline:    model.PipelineChat,
		Instruction: "Please create a file",
	}
	contexts := map[string]string{
		"tooling":     "tooling-context",
		"environment": "environment-context",
		"analyzer":    "analyzer-context",
	}
	prompt := buildAutonomousRewritePrompt(job, contexts, "NEED_INPUT: what filename?", "what filename?")
	if !strings.Contains(prompt, "<BLOCKED_DRAFT>\nNEED_INPUT: what filename?\n</BLOCKED_DRAFT>") {
		t.Fatalf("expected blocked draft block in rewrite prompt, got: %q", prompt)
	}
	if !strings.Contains(prompt, "If a file/document is requested but filename is missing, default to `test`.") {
		t.Fatalf("expected file default hint for file-related prompt, got: %q", prompt)
	}

	nonFileJob := model.Job{
		Pipeline:    model.PipelineChat,
		Instruction: "weather in Fredericksburg, Virginia",
	}
	nonFilePrompt := buildAutonomousRewritePrompt(nonFileJob, contexts, "NEED_INPUT: should I browse web?", "should I browse web?")
	if strings.Contains(nonFilePrompt, "If a file/document is requested but filename is missing, default to `test`.") {
		t.Fatalf("did not expect file default hint for non-file prompt, got: %q", nonFilePrompt)
	}
}

func TestPlannerActionCatalogIncludesPipelineAndHostActions(t *testing.T) {
	job := model.Job{
		Pipeline: model.PipelineChat,
		Metadata: json.RawMessage(`{
			"host_tools_available":"bash,git,go,npm,python3,docker,ffmpeg,ip,dig,nmcli",
			"host_env_package_managers":"apt-get,dpkg"
		}`),
	}
	got := plannerActionCatalog(job)
	expectContains := []string{
		"- plan: generate an execution plan JSON",
		"- web_search: fetch external information when required or time-sensitive",
		"- roleplay: draft the user-facing response",
		"- verify: validate/refine response and run tests when appropriate",
		"- internet/web access is available by default for this run",
		"- treat internet as unavailable only when tooling/environment/output indicates network failure",
		"Pipeline specialist assignments:",
		"planner_specialist",
		"filesystem_research_specialist",
		"review_verification_specialist",
		"- local_shell.run_command",
		"- repo.inspect_and_diff",
		"- repo.go_build_and_test",
		"- repo.node_dependency_and_test",
		"- repo.python_dependency_and_test",
		"- container.build_and_compose_control",
		"- media.subtitle_audio_video_processing",
		"- network.local_ip_and_open_ports_inspection",
		"- network.dns_route_whois_scan_diagnostics",
		"- network.vpn_detection_and_status",
		"- system.package_install_via_",
	}
	for _, expected := range expectContains {
		if !strings.Contains(got, expected) {
			t.Fatalf("plannerActionCatalog missing %q\nfull=%s", expected, got)
		}
	}
}

func TestIsFollowUpStatusCheckInstruction(t *testing.T) {
	if !isFollowUpStatusCheckInstruction("Is it done?", model.PipelineChat) {
		t.Fatalf("expected follow-up status check to be detected")
	}
	if isFollowUpStatusCheckInstruction("Is it done?", model.PipelineAssistant) {
		t.Fatalf("expected non-chat pipeline to be excluded")
	}
	if isFollowUpStatusCheckInstruction("Please create test document", model.PipelineChat) {
		t.Fatalf("expected non-follow-up instruction to be excluded")
	}
}

func TestParentJobID(t *testing.T) {
	job := model.Job{Metadata: json.RawMessage(`{"parent_job_id":42}`)}
	if got := parentJobID(job); got != 42 {
		t.Fatalf("parentJobID()=%d, want 42", got)
	}
}

func TestTestFilePathForJob(t *testing.T) {
	withCWD := model.Job{Metadata: json.RawMessage(`{"client_cwd":"/tmp/chat"}`)}
	wantWithCWD := filepath.Join("/tmp/chat", "test")
	if got := testFilePathForJob(withCWD); got != wantWithCWD {
		t.Fatalf("testFilePathForJob(with cwd)=%q, want %q", got, wantWithCWD)
	}

	withoutCWD := model.Job{}
	if got := testFilePathForJob(withoutCWD); got != "test" {
		t.Fatalf("testFilePathForJob(without cwd)=%q, want %q", got, "test")
	}
}

func TestTestFilePathForJobUsesRequestedFilename(t *testing.T) {
	job := model.Job{
		Instruction: "in this directory, make a demo html file",
		Metadata:    json.RawMessage(`{"client_cwd":"/tmp/chat"}`),
	}
	want := filepath.Join("/tmp/chat", "demo.html")
	if got := testFilePathForJob(job); got != want {
		t.Fatalf("testFilePathForJob(with requested filename)=%q, want %q", got, want)
	}
}

func TestVerifyTestFileCommand(t *testing.T) {
	job := model.Job{Metadata: json.RawMessage(`{"client_cwd":"/tmp/chat"}`)}
	want := "ls -l " + `"/tmp/chat/test"`
	if got := verifyTestFileCommand(job); got != want {
		t.Fatalf("verifyTestFileCommand(with cwd)=%q, want %q", got, want)
	}

	if got := verifyTestFileCommand(model.Job{}); got != "ls -l test" {
		t.Fatalf("verifyTestFileCommand(without cwd)=%q, want %q", got, "ls -l test")
	}
}

func TestMetadataCSV(t *testing.T) {
	metadata := json.RawMessage(`{"host_tools_available":"git, go,python3,go,, "}`)
	got := metadataCSV(metadata, "host_tools_available")
	want := []string{"git", "go", "python3"}
	if len(got) != len(want) {
		t.Fatalf("metadataCSV length=%d want %d values=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("metadataCSV value[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestHostToolAvailable(t *testing.T) {
	tools := map[string]struct{}{
		"git":     {},
		"python3": {},
		"node":    {},
	}
	if !hostToolAvailable("git", tools) {
		t.Fatalf("expected direct tool match")
	}
	if !hostToolAvailable("python", tools) {
		t.Fatalf("expected python alias to match python3")
	}
	if !hostToolAvailable("nodejs", tools) {
		t.Fatalf("expected nodejs alias to match node")
	}
	if hostToolAvailable("docker", tools) {
		t.Fatalf("did not expect unavailable tool to match")
	}
}

func TestTimeSensitivityHeuristics(t *testing.T) {
	if !isTimeSensitiveInstruction("latest fed decision today") {
		t.Fatalf("expected time-sensitive instruction to be detected")
	}
	if isTimeSensitiveInstruction("refactor auth service") {
		t.Fatalf("did not expect non-time-sensitive instruction to be flagged")
	}
}

func TestShouldForceFreshWebSearch(t *testing.T) {
	tests := []struct {
		name        string
		instruction string
		feedback    string
		want        bool
	}{
		{
			name:        "explicit web search request",
			instruction: "please do a web search for the latest release notes",
			want:        true,
		},
		{
			name:        "memory flagged outdated",
			instruction: "your memory is out of date; check online",
			want:        true,
		},
		{
			name:        "feedback marks memory wrong",
			instruction: "answer this question",
			feedback:    "that memory is wrong, search the web instead",
			want:        true,
		},
		{
			name:        "normal coding request",
			instruction: "refactor auth service",
			want:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldForceFreshWebSearch(tc.instruction, tc.feedback)
			if got != tc.want {
				t.Fatalf("shouldForceFreshWebSearch(%q, %q)=%v want %v", tc.instruction, tc.feedback, got, tc.want)
			}
		})
	}
}

func TestShouldBypassHistoricalContext(t *testing.T) {
	tests := []struct {
		name        string
		instruction string
		feedback    string
		want        bool
	}{
		{
			name:        "explicit ignore previous context",
			instruction: "start over and ignore previous conversation",
			want:        true,
		},
		{
			name:        "stale memory warning",
			instruction: "that memory is outdated, do not use cached context",
			want:        true,
		},
		{
			name:        "feedback requests fresh thread",
			instruction: "answer this",
			feedback:    "use a fresh thread for this turn",
			want:        true,
		},
		{
			name:        "normal request",
			instruction: "explain this diff",
			want:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldBypassHistoricalContext(tc.instruction, tc.feedback)
			if got != tc.want {
				t.Fatalf("shouldBypassHistoricalContext(%q, %q)=%v want %v", tc.instruction, tc.feedback, got, tc.want)
			}
		})
	}
}

func TestResolveHistoricalMemoryMode(t *testing.T) {
	tests := []struct {
		name     string
		metadata json.RawMessage
		want     string
	}{
		{name: "default auto", metadata: nil, want: "auto"},
		{name: "force on by string", metadata: json.RawMessage(`{"memory_retrieval":"deep"}`), want: "on"},
		{name: "force off by string", metadata: json.RawMessage(`{"memory_mode":"recent_only"}`), want: "off"},
		{name: "force on by bool", metadata: json.RawMessage(`{"historical_memory":true}`), want: "on"},
		{name: "force off by bool", metadata: json.RawMessage(`{"historical_memory":false}`), want: "off"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveHistoricalMemoryMode(tc.metadata)
			if got != tc.want {
				t.Fatalf("resolveHistoricalMemoryMode()=%q want %q", got, tc.want)
			}
		})
	}
}

func TestShouldRetrieveHistoricalMemory(t *testing.T) {
	chatJob := model.Job{
		Pipeline:    model.PipelineChat,
		Instruction: "Explain this error",
	}

	if got, reason := shouldRetrieveHistoricalMemory(chatJob, map[string]string{}); got {
		t.Fatalf("expected light chat default to skip historical retrieval, got=%v reason=%q", got, reason)
	}

	recallJob := model.Job{
		Pipeline:    model.PipelineChat,
		Instruction: "What did we discuss earlier in this chat?",
	}
	if got, reason := shouldRetrieveHistoricalMemory(recallJob, map[string]string{}); !got {
		t.Fatalf("expected recall request to enable historical retrieval, got=%v reason=%q", got, reason)
	}

	forcedOn := model.Job{
		Pipeline:    model.PipelineChat,
		Instruction: "hello",
		Metadata:    json.RawMessage(`{"memory_retrieval":"on"}`),
	}
	if got, reason := shouldRetrieveHistoricalMemory(forcedOn, map[string]string{}); !got {
		t.Fatalf("expected metadata force-on to enable historical retrieval, got=%v reason=%q", got, reason)
	}

	forcedOff := model.Job{
		Pipeline:    model.PipelineChat,
		Instruction: "What did we discuss earlier in this chat?",
		Metadata:    json.RawMessage(`{"memory_mode":"off"}`),
	}
	if got, reason := shouldRetrieveHistoricalMemory(forcedOff, map[string]string{}); got {
		t.Fatalf("expected metadata force-off to skip historical retrieval, got=%v reason=%q", got, reason)
	}

	assistantJob := model.Job{
		Pipeline:    model.PipelineAssistant,
		Instruction: "Summarize previous work",
	}
	if got, reason := shouldRetrieveHistoricalMemory(assistantJob, map[string]string{}); !got {
		t.Fatalf("expected non-chat pipeline to keep retrieval enabled, got=%v reason=%q", got, reason)
	}
}

func TestShouldRetrySameModelAfterCreateEOF(t *testing.T) {
	if !shouldRetrySameModelAfterCreateEOF(errors.New(`ollama create failed: status=500 body={"error":"EOF"}`)) {
		t.Fatalf("expected create EOF to trigger same-model retry")
	}
	if shouldRetrySameModelAfterCreateEOF(errors.New(`ollama generate failed: status=500 body={"error":"EOF"}`)) {
		t.Fatalf("did not expect non-create EOF to trigger same-model retry")
	}
}

func TestPickThinkingModelPrefersFallbackWhenFastIsImplicit(t *testing.T) {
	svc := Service{
		models: ModelRouting{
			Default:   "qwen3-coder-next",
			Fast:      "lfm2.5-thinking",
			Reasoning: "qwen3-coder-next",
		},
	}
	job := model.Job{
		Instruction: "create a test file",
		Metadata:    json.RawMessage(`{}`),
	}
	got := svc.pickThinkingModel(job, map[string]string{}, "qwen3-coder-next")
	if got != "qwen3-coder-next" {
		t.Fatalf("pickThinkingModel()=%q want %q", got, "qwen3-coder-next")
	}
}

func TestPickThinkingModelUsesFastWhenExplicit(t *testing.T) {
	svc := Service{
		models: ModelRouting{
			Default:   "qwen3-coder-next",
			Fast:      "lfm2.5-thinking",
			Reasoning: "qwen3-coder-next",
		},
	}
	job := model.Job{
		Instruction: "create a test file",
		Metadata:    json.RawMessage(`{"reasoning_level":"fast"}`),
	}
	got := svc.pickThinkingModel(job, map[string]string{}, "qwen3-coder-next")
	if got != "lfm2.5-thinking" {
		t.Fatalf("pickThinkingModel()=%q want %q", got, "lfm2.5-thinking")
	}
}

func TestResolvePreparedPromptHintTournamentScope(t *testing.T) {
	prompt := strings.Join([]string{
		"Determine whether CHUNK is relevant to GOAL and summarize only supported facts.",
		"GOAL:",
		"create test-4 in current directory",
		"SOURCE:",
		"workspace",
		"CHUNK:",
		"README mentions test files",
	}, "\n\n")

	got := resolvePreparedPromptHint(
		"tournament_leaf_summary_workspace_chunk_1",
		prompt,
		"Execute the system instructions and return the requested output only.",
	)
	if !strings.Contains(got, "Assess CHUNK relevance to GOAL") {
		t.Fatalf("resolvePreparedPromptHint() missing tournament instruction, got=%q", got)
	}
	if !strings.Contains(got, "Goal: create test-4 in current directory") {
		t.Fatalf("resolvePreparedPromptHint() missing goal context, got=%q", got)
	}
}

func TestResolvePreparedPromptHintVerifyReviseScope(t *testing.T) {
	prompt := strings.Join([]string{
		"You are revising an assistant response after verification findings.",
		"Instruction:",
		"create test-4 file in current directory",
		"Current Response:",
		"Need more context",
	}, "\n\n")

	got := resolvePreparedPromptHint(
		"verify_revise_attempt_1_of_2",
		prompt,
		"Execute the system instructions and return the requested output only.",
	)
	if !strings.Contains(got, "Revise the assistant response using verification findings") {
		t.Fatalf("resolvePreparedPromptHint() missing revise instruction, got=%q", got)
	}
	if !strings.Contains(got, "Instruction: create test-4 file in current directory") {
		t.Fatalf("resolvePreparedPromptHint() missing instruction context, got=%q", got)
	}
}

func TestResolvePreparedPromptHintActivatesV3SpecialistInvocation(t *testing.T) {
	prompt := strings.Join([]string{
		"Return JSON only using the required response envelope.",
		"SPECIALIST_INVOCATION:",
		`{"role_id":"prompt_interpreter","objective":"interpret the current request"}`,
	}, "\n\n")

	got := resolvePreparedPromptHint("v3_intent_parse", prompt, "Return only the requested output.")
	if got != "Begin the control-plane-assigned work now." {
		t.Fatalf("resolvePreparedPromptHint()=%q", got)
	}
}

func TestResolvePreparedPromptHintDeliversV3DirectFeedback(t *testing.T) {
	prompt := strings.Join([]string{
		"Execute the typed objective.",
		"SPECIALIST_INVOCATION:",
		`{"role_id":"subtask_executor"}`,
		promptBlock("DIRECT_FEEDBACK", "Your last patch assumed the wrong go.mod content. Leave go.mod unchanged and patch the application files now."),
	}, "\n\n")

	got := resolvePreparedPromptHint("v3_subtask_tool_3", prompt, "Return only the requested output.")
	if got != "Your last patch assumed the wrong go.mod content. Leave go.mod unchanged and patch the application files now." {
		t.Fatalf("resolvePreparedPromptHint()=%q", got)
	}
}

func TestV3OutputTokenBudgetsReserveLargeOutputForToolExecution(t *testing.T) {
	cases := map[string]int{
		"v3_intent_parse":                   2048,
		"v3_planning":                       2048,
		"v3_verification":                   2048,
		"v3_analysis":                       1024,
		"v3_response_draft":                 1024,
		"v3_subtask_tool_1":                 4096,
		"v3_subtask_tool_2_contract_repair": 4096,
		"legacy_analysis":                   0,
	}
	for scope, want := range cases {
		if got := v3OutputTokenBudget(scope); got != want {
			t.Errorf("v3OutputTokenBudget(%q)=%d want %d", scope, got, want)
		}
	}
}

func TestV3ScopesRequireProviderEnforcedJSON(t *testing.T) {
	for _, scope := range []string{"v3_intent_parse", "v3_implementation_manifest", "v3_work_item_writer_main_1", "v3_verification"} {
		if got := responseFormatForScope(scope); got != llm.ResponseFormatJSON {
			t.Fatalf("responseFormatForScope(%q)=%q, want JSON", scope, got)
		}
	}
	if got := responseFormatForScope("legacy_analysis"); got != "" {
		t.Fatalf("legacy response format=%q, want text", got)
	}
}

func TestV3SubtaskBudgetAllowsImplementVerifyRepairCycle(t *testing.T) {
	if maxSubtaskToolTurns < 20 {
		t.Fatalf("maxSubtaskToolTurns=%d, want at least 20", maxSubtaskToolTurns)
	}
	if maxSubtaskToolCalls < 24 {
		t.Fatalf("maxSubtaskToolCalls=%d, want at least 24", maxSubtaskToolCalls)
	}
	if maxToolCallsPerTurn != 1 {
		t.Fatalf("maxToolCallsPerTurn=%d, want immediate feedback after each action", maxToolCallsPerTurn)
	}
}
