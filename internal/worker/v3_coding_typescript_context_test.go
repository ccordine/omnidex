package worker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGenericFiveCapabilityWorkerKeepsExecutableAuthorityInsideThePortableEnvelope(t *testing.T) {
	t.Parallel()

	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: "interactive workshop",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "multiple work areas"},
			{ID: "requirement_002", SourceQuote: "custom controls"},
			{ID: "requirement_003", SourceQuote: "direct input"},
			{ID: "requirement_004", SourceQuote: "saved presets"},
			{ID: "requirement_005", SourceQuote: "live output"},
		},
	}
	_, blueprint, _, err := compileGenericTypeScriptBrowserBlueprint(
		"bounded", specification, genericBrowserSkillBindings(specification),
		genericBrowserWorkload(t, specification),
		genericBrowserCapabilityBindings(specification),
	)
	if err != nil {
		t.Fatal(err)
	}
	feature, exists := directCodingTypeScriptBlueprintBlock(blueprint, "feature.001")
	if !exists {
		t.Fatal("feature.001 is missing")
	}
	runtimeAPI, exists := directCodingTypeScriptBlueprintBlock(blueprint, "runtime.api")
	if !exists {
		t.Fatal("runtime.api is missing")
	}
	job, err := newDirectCodingTypeScriptPortableJob(directCodingTypeScriptFragmentJob{
		block: feature, tsx: true, available: runtimeAPI.API,
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompt) > 16*1024 {
		t.Fatalf("five-capability worker prompt=%dB exceeds portable envelope", len(prompt))
	}
	for _, required := range []string{
		string(specification.Surface), specification.ProductQuote,
		"multiple work areas", "Implement interactive behavior for multiple work areas.",
		"Expose an accessible user control for multiple work areas.",
		"Using the control produces the requested visible state change.",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("five-capability worker omitted executable authority %q:\n%s", required, prompt)
		}
	}
	if strings.Contains(prompt, "createApplicationRuntime") {
		t.Fatalf("feature worker received the application factory it cannot use:\n%s", prompt)
	}
}

func TestGenericWorkersReceiveOnlyLocalAuthorityAndCodeOwnedCapabilityAPIs(t *testing.T) {
	t.Parallel()

	specification := genericBrowserSpecification()
	program, err := compileDirectCodingProgram(
		"unseen", specification, nil, genericBrowserSkillBindings(specification),
		genericBrowserWorkload(t, specification),
		genericBrowserCapabilityBindings(specification),
	)
	if err != nil {
		t.Fatal(err)
	}
	var mutex sync.Mutex
	prompts := make([]string, 0, 2)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1, MaxConcurrency: 2,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string, _ map[string]any) (string, error) {
			mutex.Lock()
			prompts = append(prompts, prompt)
			mutex.Unlock()
			const marker = "The declaration must match this signature exactly:\n"
			_, remainder, found := strings.Cut(prompt, marker)
			if !found {
				return "", fmt.Errorf("prompt has no code-owned function signature")
			}
			signature, _, _ := strings.Cut(remainder, "\n")
			if strings.HasPrefix(signature, "async function VerifyFeature") {
				return signature + ` { expect(screen.getByText('ready')).not.toBeNull(); }`, nil
			}
			return signature + ` { return <button onClick={() => actions.set('ready', true)}>{String(state.ready ?? 'ready')}</button>; }`, nil
		}),
	}
	input := applicationWorkloadInput(specification)
	err = runDirectCodingApplicationTaskLifecycle(
		input, program.Workload, &program,
		directCodingApplicationTaskLifecycleHooks{
			BuildBlock: func(
				_ assemblyline.ApplicationTaskContext,
				stage *directCodingProgram,
				block assemblyline.TypeScriptBlock,
			) (string, error) {
				job, jobErr := directCodingApplicationTaskFragmentJob(stage, block)
				if jobErr != nil {
					return "", jobErr
				}
				return runDirectCodingTypeScriptFragmentWorker(runtime, "coder", job)
			},
			Verify:     func(assemblyline.ApplicationTaskContext, *directCodingProgram) error { return nil },
			FinalStage: func(*directCodingProgram) error { return nil },
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Generated) != 4 || len(prompts) != 4 {
		t.Fatalf("generated=%d prompts=%d", len(program.Generated), len(prompts))
	}
	for _, prompt := range prompts {
		containsFirst := strings.Contains(prompt, "Exact user requirement: filter the catalog")
		containsSecond := strings.Contains(prompt, "Exact user requirement: remember my selection")
		if containsFirst == containsSecond {
			t.Fatalf("worker prompt did not contain exactly one local authority:\n%s", prompt)
		}
		implementationPrompt := strings.Contains(prompt, "exactly:\nfunction Feature001View") ||
			strings.Contains(prompt, "exactly:\nfunction Feature002View")
		if implementationPrompt {
			for _, required := range []string{"interface FeatureActions", "ViewProps"} {
				if !strings.Contains(prompt, required) {
					t.Fatalf("implementation worker omitted code-owned view API %q:\n%s", required, prompt)
				}
			}
			for _, forbidden := range []string{"playTone", "useGlobalKeyboard", "FeatureRuntime"} {
				if strings.Contains(prompt, forbidden) {
					t.Fatalf("implementation worker received code-owned lifecycle %q:\n%s", forbidden, prompt)
				}
			}
			if strings.Contains(prompt, "Exact user requirement: filter the catalog") &&
				(strings.Contains(prompt, "capability_001") || strings.Contains(prompt, "capability_002")) {
				t.Fatalf("independent feature received an undeclared capability:\n%s", prompt)
			}
			if strings.Contains(prompt, "Exact user requirement: remember my selection") &&
				(!strings.Contains(prompt, "capability_001") || strings.Contains(prompt, "capability_002")) {
				t.Fatalf("dependent feature received anything beyond its direct capability:\n%s", prompt)
			}
		}
		for _, document := range program.TypeScript.Documents {
			if strings.Contains(prompt, document.Path) || strings.Contains(prompt, document.ID) {
				t.Fatalf("fragment prompt exposed document identity %s/%s:\n%s", document.ID, document.Path, prompt)
			}
		}
		for _, forbidden := range []string{"dependency graph", "workspace", "project tree", "filename", "benchmark"} {
			if strings.Contains(strings.ToLower(prompt), forbidden) {
				t.Fatalf("fragment prompt exposed %q:\n%s", forbidden, prompt)
			}
		}
	}
}

func TestGenerationAndAcceptancePromptsReceiveTheAcceptedExecutableJobAndOnlyRelatedSiblings(t *testing.T) {
	t.Parallel()

	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "catalog browser",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "filter the catalog"},
			{ID: "requirement_002", SourceQuote: "remember my selection"},
			{ID: "requirement_003", SourceQuote: "export a report"},
		},
	}
	workload := genericBrowserWorkload(t, specification)
	capabilities := directCodingCapabilityGraph{
		"requirement_001": nil,
		"requirement_002": {{
			RequirementID: "requirement_001",
			CapabilityID:  "capability_001",
			Purpose:       "filter the catalog",
		}},
		"requirement_003": nil,
	}
	program, err := compileDirectCodingProgram(
		"unseen", specification, nil, genericBrowserSkillBindings(specification), workload, capabilities,
	)
	if err != nil {
		t.Fatal(err)
	}
	input := applicationWorkloadInput(specification)
	context, err := assemblyline.ProjectApplicationTaskContext(input, workload, "task_002")
	if err != nil {
		t.Fatal(err)
	}
	stage, err := projectDirectCodingApplicationTaskStage(program, context)
	if err != nil {
		t.Fatal(err)
	}

	feature, exists := directCodingTypeScriptBlueprintBlock(stage.TypeScript, "feature.002")
	if !exists {
		t.Fatal("feature.002 is missing")
	}
	generationPrompt := renderApplicationTaskFragmentPrompt(t, &stage, feature)
	stage.Generated[feature.ID] = feature.Signature +
		` { return <button onClick={() => actions.set('selected', true)}>Remember selection</button>; }`
	acceptance, exists := directCodingTypeScriptBlueprintBlock(stage.TypeScript, "acceptance.002")
	if !exists {
		t.Fatal("acceptance.002 is missing")
	}
	acceptancePrompt := renderApplicationTaskFragmentPrompt(t, &stage, acceptance)

	task := workload.Tasks[1]
	for label, prompt := range map[string]string{
		"generation": generationPrompt,
		"acceptance": acceptancePrompt,
	} {
		for _, required := range append(
			[]string{
				string(specification.Surface),
				specification.ProductQuote,
				task.RequirementQuote,
				task.Objective,
			},
			append(append([]string{}, task.RequiredBehaviors...), task.AcceptanceCriteria...)...,
		) {
			if !strings.Contains(prompt, required) {
				t.Fatalf("%s prompt omitted accepted executable-job fact %q:\n%s", label, required, prompt)
			}
		}
		for _, requiredSibling := range []string{"filter the catalog", "capability_001"} {
			if !strings.Contains(prompt, requiredSibling) {
				t.Fatalf("%s prompt omitted relation-selected sibling %q:\n%s", label, requiredSibling, prompt)
			}
		}
		for _, unrelated := range []string{
			"export a report", "capability_003",
			"Implement interactive behavior for export a report.",
			"Expose an accessible user control for export a report.",
			"The control for export a report is visible and operable.",
		} {
			if strings.Contains(prompt, unrelated) {
				t.Fatalf("%s prompt exposed unrelated sibling state %q:\n%s", label, unrelated, prompt)
			}
		}
		for _, forbidden := range []string{
			"requirement_001", "requirement_002", "requirement_003",
			"task_001", "task_002", "task_003", workload.SHA256,
			"src/features/", "depends_on", "next_task", "completion_state",
		} {
			if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
				t.Fatalf("%s prompt exposed code-owned execution state %q:\n%s", label, forbidden, prompt)
			}
		}
	}

	const publicWrapper = "function Feature002({ runtime }: FeatureProps): ReactElement"
	for _, forbidden := range []string{
		publicWrapper, "function Feature002View", "createApplicationRuntime", "createFeatureRuntime",
	} {
		if strings.Contains(acceptancePrompt, forbidden) {
			t.Fatalf("observation-only acceptance prompt exposed render authority %q:\n%s", forbidden, acceptancePrompt)
		}
	}
	if !strings.Contains(acceptancePrompt, "ALREADY_IN_SCOPE_IDENTIFIERS:\nfireEvent, screen, waitFor, expect") {
		t.Fatalf("acceptance prompt omitted its closed observation surface:\n%s", acceptancePrompt)
	}
	if strings.Contains(generationPrompt, publicWrapper) {
		t.Fatalf("implementation leaf received its code-owned public wrapper:\n%s", generationPrompt)
	}
}

func renderApplicationTaskFragmentPrompt(
	t *testing.T,
	stage *directCodingProgram,
	block assemblyline.TypeScriptBlock,
) string {
	t.Helper()
	fragment, err := directCodingApplicationTaskFragmentJob(stage, block)
	if err != nil {
		t.Fatal(err)
	}
	job, err := newDirectCodingTypeScriptPortableJob(fragment)
	if err != nil {
		t.Fatal(err)
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	return prompt
}
