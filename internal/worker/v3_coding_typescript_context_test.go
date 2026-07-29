package worker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGenericFiveCapabilityWorkerStaysInsideTheHardInitialEnvelope(t *testing.T) {
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
	if len(prompt) > 3*1024 {
		t.Fatalf("five-capability worker prompt=%dB exceeds hard initial envelope", len(prompt))
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
				name := strings.TrimPrefix(signature, "async function Verify")
				name, _, _ = strings.Cut(name, "(")
				sequence := strings.TrimPrefix(name, "Feature")
				return signature + ` { render(<` + name + ` runtime={createFeatureRuntime(createApplicationRuntime(), 'capability_` + sequence + `')} />); expect(screen.getByText('ready')).not.toBeNull(); }`, nil
			}
			return signature + ` { return <button onClick={() => actions.set('ready', true)}>{String(state.ready ?? 'ready')}</button>; }`, nil
		}),
	}
	generated, err := generateDirectCodingTypeScriptFragments(runtime, "coder", program.TypeScript)
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != 4 || len(prompts) != 4 {
		t.Fatalf("generated=%d prompts=%d", len(generated), len(prompts))
	}
	for _, prompt := range prompts {
		containsFirst := strings.Contains(prompt, "Exact feature: filter the catalog") ||
			strings.Contains(prompt, "exact accepted feature: filter the catalog")
		containsSecond := strings.Contains(prompt, "Exact feature: remember my selection") ||
			strings.Contains(prompt, "exact accepted feature: remember my selection")
		if containsFirst == containsSecond {
			t.Fatalf("worker prompt did not contain exactly one local authority:\n%s", prompt)
		}
		if strings.Contains(prompt, "READABLE_CAPABILITY_CHANNELS") {
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
			if strings.Contains(prompt, "Exact feature: filter the catalog") &&
				(strings.Contains(prompt, "capability_001") || strings.Contains(prompt, "capability_002")) {
				t.Fatalf("independent feature received an undeclared capability:\n%s", prompt)
			}
			if strings.Contains(prompt, "Exact feature: remember my selection") &&
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
