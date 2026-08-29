package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestProjectStackSelectionUsesBoundedConstraintAndCodeOwnedDefault(t *testing.T) {
	specification := testProjectStackSpecification(assemblyline.ApplicationSurfaceBrowser)
	request := "Build a browser inventory application that shows current inventory."
	for _, testCase := range []struct {
		name      string
		selection string
		wantError string
	}{
		{name: "explicit candidate", selection: "STACK_CANDIDATE_1"},
		{name: "unconstrained", selection: assemblyline.ApplicationProjectStackUnconstrained},
		{name: "unsupported", selection: assemblyline.ApplicationProjectStackUnsupported, wantError: "unsupported or contradictory"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			calls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: testPortableExecutor(func(_ string, model, prompt string) (string, error) {
					calls++
					if model != "constraint-model" {
						t.Fatalf("model=%q", model)
					}
					if strings.Contains(prompt, genericTypeScriptBrowserAdapter) || strings.Contains(prompt, "package.json") {
						t.Fatalf("constraint prompt exposed code-owned stack identity: %s", prompt)
					}
					return testCase.selection, nil
				}),
			}
			selection, err := selectDirectCodingProject(
				runtime, func() (string, error) { return "constraint-model", nil },
				request, specification, nil, nil,
			)
			stack := selection.Stack
			if testCase.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
					t.Fatalf("stack=%+v error=%v", stack, err)
				}
				return
			}
			if err != nil || stack.ID != genericTypeScriptBrowserAdapter || calls != 1 {
				t.Fatalf("stack=%+v calls=%d error=%v", stack, calls, err)
			}
		})
	}
}

func TestProjectStackConstraintPromptUsesOnlyRequestAndTechnicalCandidates(t *testing.T) {
	const request = "Build a compact command-line utility using Go."
	specification := testProjectStackSpecification(assemblyline.ApplicationSurfaceCommandLine)
	specification.ProductQuote = "PRODUCT_CONTEXT_MUST_NOT_REACH_STACK_SELECTION"
	specification.Requirements[0].SourceQuote = "REQUIREMENT_PROJECTION_MUST_NOT_REACH_STACK_SELECTION"
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_, _, prompt string) (string, error) {
			for _, required := range []string{request, "packaging shape", "STACK_CANDIDATE_1"} {
				if !strings.Contains(prompt, required) {
					t.Fatalf("stack prompt omitted %q: %s", required, prompt)
				}
			}
			for _, forbidden := range []string{
				specification.ProductQuote,
				specification.Requirements[0].SourceQuote,
				"product_context", "accepted_requirements",
			} {
				if strings.Contains(prompt, forbidden) {
					t.Fatalf("stack prompt contained forbidden projection %q: %s", forbidden, prompt)
				}
			}
			return "STACK_CANDIDATE_1", nil
		}),
	}
	selection, err := selectDirectCodingProject(
		runtime, func() (string, error) { return "constraint-model", nil },
		request, specification, nil, nil,
	)
	if err != nil || selection.Stack.ID != genericGoCommandLineAdapter {
		t.Fatalf("selection=%+v error=%v", selection, err)
	}
}

func TestProjectStackSelectionChecksExistingManifestAgainstImmutableRequest(t *testing.T) {
	specification := testProjectStackSpecification(assemblyline.ApplicationSurfaceBrowser)
	manifest := map[string]string{"package.json": typeScriptVersionProfileManifestFixture(t)}
	calls := 0
	selection, err := selectDirectCodingProject(
		typedWorkerRuntime{
			Context: context.Background(), MaxAttempts: 1,
			Execute: testPortableExecutor(func(_, _, prompt string) (string, error) {
				calls++
				if strings.Count(prompt, "STACK_CANDIDATE_") != 1 ||
					!strings.Contains(prompt, "Build a browser inventory application") {
					t.Fatalf("manifest constraint prompt lost its one compatible candidate or request: %s", prompt)
				}
				return assemblyline.ApplicationProjectStackUnconstrained, nil
			}),
		}, func() (string, error) { return "constraint-model", nil },
		"Build a browser inventory application.", specification, manifest, nil,
	)
	if err != nil || selection.Stack.ID != genericTypeScriptBrowserAdapter ||
		selection.VersionProfileID != typeScriptBrowserVersionProfileV1 || calls != 1 {
		t.Fatalf("selection=%+v calls=%d error=%v", selection, calls, err)
	}

	_, err = selectDirectCodingProject(
		typedWorkerRuntime{
			Context: context.Background(), MaxAttempts: 1,
			Execute: testPortableExecutor(func(_, _, _ string) (string, error) {
				return assemblyline.ApplicationProjectStackUnsupported, nil
			}),
		}, func() (string, error) { return "constraint-model", nil },
		"Build the browser application using an incompatible technical format.",
		specification, manifest, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported or contradictory") {
		t.Fatalf("manifest conflict error=%v", err)
	}
}

func TestProjectStackSelectionUsesRegisteredPHPServiceStack(t *testing.T) {
	specification := testProjectStackSpecification(assemblyline.ApplicationSurfaceService)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string) (string, error) {
			if strings.Contains(prompt, genericPHPServiceAdapter) ||
				!strings.Contains(prompt, "PHP with NGINX") {
				t.Fatalf("service stack prompt leaked identity or omitted technical candidate: %s", prompt)
			}
			return "STACK_CANDIDATE_1", nil
		}),
	}
	selection, err := selectDirectCodingProject(
		runtime, func() (string, error) { return "constraint-model", nil },
		"Build a PHP service.", specification, nil, nil,
	)
	if err != nil || selection.Stack.ID != genericPHPServiceAdapter ||
		selection.VersionProfileID != phpServiceVersionProfileV1 {
		t.Fatalf("selection=%+v error=%v", selection, err)
	}
}

func TestGreenfieldProjectStackSelectionUsesExplicitRegisteredNondefaultVersionProfile(t *testing.T) {
	base := requireDirectCodingVersionProfile(t, goCommandLineVersionProfileV1)
	future := syntheticFutureGoVersionProfile(base)
	qualifications := registeredDirectCodingParserQualifications()
	futureQualification := cloneTestParserQualification(
		t, qualifications, base.ParserQualification,
	)
	futureQualification.ID = "go-parser-go1.25-greenfield-test"
	futureQualification.SourceDialects = []string{future.SourceDialect}
	future.ParserQualification = futureQualification.ID
	if err := future.ValidateDefinition(future); err != nil {
		t.Fatalf("synthetic profile definition: %v", err)
	}
	profiles := append(registeredDirectCodingProjectVersionProfiles(), future)
	qualifications = append(qualifications, futureQualification)
	if err := validateDirectCodingArtifactRegistriesFrom(
		registeredDirectCodingArtifactAdapters(), profiles,
		registeredDirectCodingProjectStacks(), qualifications,
	); err != nil {
		t.Fatalf("synthetic selection registry: %v", err)
	}
	specification := testProjectStackSpecification(assemblyline.ApplicationSurfaceCommandLine)
	specification.Requirements[0].SourceQuote = "Use Go 1.25.0 for the command-line application"
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string) (string, error) {
			calls++
			for _, required := range []string{"Go 1.24.0", "Go 1.25.0", "go manifest", "1.26.0"} {
				if !strings.Contains(prompt, required) {
					t.Fatalf("version-aware constraint prompt omitted %q: %s", required, prompt)
				}
			}
			for _, forbidden := range []string{
				genericGoCommandLineAdapter, base.ID, future.ID, future.ParserQualification, "go.mod",
			} {
				if strings.Contains(prompt, forbidden) {
					t.Fatalf("constraint prompt exposed internal identity or path %q: %s", forbidden, prompt)
				}
			}
			return "STACK_CANDIDATE_2", nil
		}),
	}
	selection, err := selectDirectCodingProjectFromRegistries(
		runtime, func() (string, error) { return "constraint-model", nil },
		"Build the command-line application using Go 1.25.0.",
		specification, nil, nil, registeredDirectCodingProjectStacks(), profiles,
	)
	if err != nil || calls != 1 || selection.Stack.ID != genericGoCommandLineAdapter ||
		selection.VersionProfileID != future.ID {
		t.Fatalf("selection=%+v calls=%d error=%v", selection, calls, err)
	}
}

func TestGreenfieldProjectStackSelectionUnconstrainedUsesCodeOwnedDefaultProfile(t *testing.T) {
	base := requireDirectCodingVersionProfile(t, goCommandLineVersionProfileV1)
	future := syntheticFutureGoVersionProfile(base)
	profiles := append(registeredDirectCodingProjectVersionProfiles(), future)
	specification := testProjectStackSpecification(assemblyline.ApplicationSurfaceCommandLine)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string) (string, error) {
			calls++
			if !strings.Contains(prompt, "Go 1.25.0") {
				t.Fatalf("constraint prompt omitted registered nondefault profile: %s", prompt)
			}
			return assemblyline.ApplicationProjectStackUnconstrained, nil
		}),
	}
	selection, err := selectDirectCodingProjectFromRegistries(
		runtime, func() (string, error) { return "constraint-model", nil },
		"Build a command-line application.",
		specification, nil, nil, registeredDirectCodingProjectStacks(), profiles,
	)
	if err != nil || calls != 1 || selection.Stack.ID != genericGoCommandLineAdapter ||
		selection.VersionProfileID != goCommandLineVersionProfileV1 {
		t.Fatalf("selection=%+v calls=%d error=%v", selection, calls, err)
	}
}

func testProjectStackSpecification(surface assemblyline.ApplicationSurface) assemblyline.ApplicationSpecification {
	return assemblyline.ApplicationSpecification{
		Surface: surface, ProductQuote: "inventory application",
		Requirements: []assemblyline.Requirement{{ID: "requirement_001", SourceQuote: "Show current inventory"}},
	}
}

func typeScriptVersionProfileManifestFixture(t *testing.T) string {
	t.Helper()
	files, err := typeScriptBrowserStaticFiles(
		requireDirectCodingVersionProfile(t, typeScriptBrowserVersionProfileV1),
		"version-selection", "Version selection", "main { display: block; }",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Path == "package.json" {
			return file.Content
		}
	}
	t.Fatal("TypeScript version fixture lacks package.json")
	return ""
}
