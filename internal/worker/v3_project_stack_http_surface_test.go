package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestBrowserClassificationWithExplicitLaravelConstraintCompilesLaravelHTTPProgram(t *testing.T) {
	t.Parallel()

	const authority = "Build an interactive browser application using Laravel 13 with server-rendered HTML."
	laravelCandidate := browserLaravelCandidateID(t)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, model, prompt string, _ map[string]any) (string, error) {
			switch model {
			case "surface-model":
				return `{"schema":"omnidex.application-class.v1","surface":"browser_application"}`, nil
			case "constraint-model":
				for _, required := range []string{
					"Use Laravel 13 with server-rendered HTML.", "TypeScript with React",
					"PHP with NGINX", "Laravel 13 with server rendering",
				} {
					if !strings.Contains(prompt, required) {
						t.Fatalf("browser format prompt omitted %q: %s", required, prompt)
					}
				}
				return fmt.Sprintf(
					`{"schema":"%s","candidate_id":%q}`,
					assemblyline.ApplicationProjectStackConstraintSchemaV1, laravelCandidate,
				), nil
			default:
				return "", fmt.Errorf("unexpected model %q", model)
			}
		}),
	}
	classification, err := classifyApplicationSurface(runtime, "surface-model", authority, nil)
	if err != nil {
		t.Fatal(err)
	}
	specification := assemblyline.ApplicationSpecification{
		Surface: classification.Surface, ProductQuote: "server-rendered browser application",
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: "Use Laravel 13 with server-rendered HTML.",
		}},
	}
	if err := specification.Validate(); err != nil {
		t.Fatal(err)
	}
	selection, err := selectDirectCodingProject(
		runtime, func() (string, error) { return "constraint-model", nil },
		specification, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Stack.ID != laravelHTTPServiceAdapter ||
		selection.VersionProfileID != laravelVersionProfileV1 {
		t.Fatalf("browser Laravel selection=%+v", selection)
	}

	workload := browserHTTPWorkload(t, specification)
	target, coverage, err := resolveDirectCodingTargetTree(
		typedWorkerRuntime{}, "", "", specification, workload, selection.Stack, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	target.VersionProfileID = selection.VersionProfileID
	capabilities := directCodingCapabilityGraph{"requirement_001": nil}
	endpoints := directCodingServiceEndpointPlan{
		WorkloadSHA256: workload.SHA256, ProductContext: specification.ProductQuote,
		Requirements: map[string]assemblyline.ApplicationServiceEndpointRequirement{
			workload.Tasks[0].ID: assemblyline.ApplicationServiceEndpointRequired,
		},
		ByTask: map[string]assemblyline.ApplicationServiceEndpointContract{
			workload.Tasks[0].ID: {
				Schema:   assemblyline.ApplicationServiceEndpointContractSchemaV1,
				Exposure: assemblyline.ApplicationServiceEndpointPublic,
				Method:   assemblyline.ApplicationServiceEndpointGET, RouteTemplate: "/",
				RequestMedia:  assemblyline.ApplicationServiceEndpointMediaNone,
				ResponseMedia: assemblyline.ApplicationServiceEndpointHTML, SuccessStatus: 200,
			},
		},
	}
	program, err := compileDirectCodingServiceProgram(
		"browser-http", specification, nil, map[string]directCodingSkillBinding{}, workload,
		capabilities, target, coverage, testRequestLocalServiceStatePlan(workload), endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	files := directCodingFileTaskMap(program.StaticFiles)
	if program.StackID != laravelHTTPServiceAdapter || files["artisan"] == "" ||
		files["composer.lock"] == "" || files["docker-compose.yml"] == "" {
		t.Fatalf("compiled browser Laravel program lost its selected stack: %+v", program)
	}
}

func TestProjectStackSurfaceSetsPreserveExactDefaults(t *testing.T) {
	t.Parallel()

	assertProjectStackIDs(t, assemblyline.ApplicationSurfaceBrowser,
		genericTypeScriptBrowserAdapter, genericPHPServiceAdapter, laravelHTTPServiceAdapter)
	assertProjectStackIDs(t, assemblyline.ApplicationSurfaceService,
		genericPHPServiceAdapter, laravelHTTPServiceAdapter)
	assertProjectStackIDs(t, assemblyline.ApplicationSurfaceCommandLine,
		genericGoCommandLineAdapter, genericJavaScriptCommandLineAdapter,
		genericRustCommandLineAdapter, genericJavaCommandLineAdapter)

	for surface, want := range map[assemblyline.ApplicationSurface]string{
		assemblyline.ApplicationSurfaceBrowser:     genericTypeScriptBrowserAdapter,
		assemblyline.ApplicationSurfaceCommandLine: genericGoCommandLineAdapter,
		assemblyline.ApplicationSurfaceService:     genericPHPServiceAdapter,
	} {
		stacks := directCodingProjectStacksForSurface(surface)
		selection, err := directCodingDefaultProjectSelection(
			surface, stacks, registeredDirectCodingProjectVersionProfiles(),
		)
		if err != nil || selection.Stack.ID != want {
			t.Fatalf("surface %s default=%+v want=%s error=%v", surface, selection, want, err)
		}
	}
}

func TestProjectStackSurfaceSetsRejectInvalidRegistration(t *testing.T) {
	t.Parallel()

	base, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*directCodingProjectStack){
		"empty supported set": func(stack *directCodingProjectStack) {
			stack.SupportedSurfaces = nil
		},
		"duplicate supported surface": func(stack *directCodingProjectStack) {
			stack.SupportedSurfaces = []assemblyline.ApplicationSurface{
				assemblyline.ApplicationSurfaceBrowser,
				assemblyline.ApplicationSurfaceBrowser,
			}
		},
		"unordered supported surfaces": func(stack *directCodingProjectStack) {
			stack.SupportedSurfaces = []assemblyline.ApplicationSurface{
				assemblyline.ApplicationSurfaceService,
				assemblyline.ApplicationSurfaceBrowser,
			}
		},
		"unsupported surface": func(stack *directCodingProjectStack) {
			stack.SupportedSurfaces = []assemblyline.ApplicationSurface{
				assemblyline.ApplicationSurfaceUnsupported,
			}
		},
		"default outside supported set": func(stack *directCodingProjectStack) {
			stack.DefaultSurfaces = []assemblyline.ApplicationSurface{
				assemblyline.ApplicationSurfaceService,
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			stack := base
			mutate(&stack)
			if err := validateDirectCodingProjectStackSurfaceSet(stack); err == nil {
				t.Fatalf("accepted invalid surface registration: %+v", stack)
			}
		})
	}

	stacks := registeredDirectCodingProjectStacks()
	for index := range stacks {
		if stacks[index].ID == genericPHPServiceAdapter {
			stacks[index].DefaultSurfaces = []assemblyline.ApplicationSurface{
				assemblyline.ApplicationSurfaceBrowser,
				assemblyline.ApplicationSurfaceService,
			}
		}
	}
	if err := validateDirectCodingProjectStackDefaults(stacks); err == nil ||
		!strings.Contains(err.Error(), "browser_application has 2 default") {
		t.Fatalf("duplicate browser default error=%v", err)
	}
}

func browserLaravelCandidateID(t *testing.T) string {
	t.Helper()
	formats, err := directCodingProjectFormatCandidates(
		directCodingProjectStacksForSurface(assemblyline.ApplicationSurfaceBrowser),
		registeredDirectCodingProjectVersionProfiles(),
	)
	if err != nil {
		t.Fatal(err)
	}
	for index, format := range formats {
		if format.Stack.ID == laravelHTTPServiceAdapter {
			return fmt.Sprintf("STACK_CANDIDATE_%d", index+1)
		}
	}
	t.Fatal("browser surface omitted the registered Laravel format")
	return ""
}

func browserHTTPWorkload(
	t *testing.T,
	specification assemblyline.ApplicationSpecification,
) assemblyline.FrozenApplicationWorkload {
	t.Helper()
	workload, err := assemblyline.FreezeApplicationWorkload(
		applicationWorkloadInput(specification), assemblyline.ApplicationWorkloadDraft{
			Schema: assemblyline.ApplicationWorkloadDraftSchemaV1,
			Tasks: []assemblyline.ApplicationWorkloadTaskDraft{{
				RequirementID:      "requirement_001",
				Objective:          "Return one server-rendered response.",
				RequiredBehaviors:  []string{"Produce one observable HTML result."},
				AcceptanceCriteria: []string{"The response contains the observable result."},
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return workload
}

func assertProjectStackIDs(
	t *testing.T,
	surface assemblyline.ApplicationSurface,
	want ...string,
) {
	t.Helper()
	stacks := directCodingProjectStacksForSurface(surface)
	got := make([]string, len(stacks))
	for index, stack := range stacks {
		got[index] = stack.ID
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("surface %s stacks=%v want=%v", surface, got, want)
	}
}

func directCodingFileTaskMap(files []directCodingFileTask) map[string]string {
	result := make(map[string]string, len(files))
	for _, file := range files {
		result[file.Path] = file.Content
	}
	return result
}
