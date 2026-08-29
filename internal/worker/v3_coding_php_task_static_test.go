package worker

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPHPTaskStaticProjectionLimitsContainerInputsToFocusedTask(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		responseMedia assemblyline.ApplicationServiceEndpointMedia
		copyInputs    []string
		dockerignore  string
	}{
		{
			name:          "HTML keeps its conditional Tailwind inputs",
			responseMedia: assemblyline.ApplicationServiceEndpointHTML,
			copyInputs: []string{
				"composer.json", "package-lock.json", "package.json", "resources/styles.css",
				"src/Feature101.php", "src/Runtime.php", "tests/Feature101Test.php",
			},
			dockerignore: "**\n" +
				"!composer.json\n!package-lock.json\n!package.json\n!resources\n" +
				"!resources/styles.css\n!src\n!src/Feature101.php\n!src/Runtime.php\n" +
				"!tests\n!tests/Feature101Test.php\n",
		},
		{
			name:          "JSON omits the unused Tailwind inputs",
			responseMedia: assemblyline.ApplicationServiceEndpointJSON,
			copyInputs: []string{
				"composer.json", "src/Feature101.php", "src/Runtime.php",
				"tests/Feature101Test.php",
			},
			dockerignore: "**\n" +
				"!composer.json\n!src\n!src/Feature101.php\n!src/Runtime.php\n" +
				"!tests\n!tests/Feature101Test.php\n",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			program := phpTaskStaticProjectionFixture(t, testCase.responseMedia)
			context, err := assemblyline.ProjectApplicationTaskContext(
				program.Workload, program.Workload.Tasks[0].ID,
			)
			if err != nil {
				t.Fatal(err)
			}
			stage, err := projectDirectCodingApplicationTaskStage(program, context)
			if err != nil {
				t.Fatal(err)
			}

			if got := phpDockerfileContextCopyInputs(
				t, phpServiceFileContent(t, stage.StaticFiles, "Dockerfile"),
			); !reflect.DeepEqual(got, testCase.copyInputs) {
				t.Fatalf("focused PHP Docker COPY inputs=%v want=%v", got, testCase.copyInputs)
			}
			if got := phpServiceFileContent(t, stage.StaticFiles, ".dockerignore"); got != testCase.dockerignore {
				t.Fatalf("focused PHP .dockerignore:\n%s\nwant:\n%s", got, testCase.dockerignore)
			}
			for _, forbidden := range []string{
				"public/index.php", "tests/HttpVerifier.php", "tests/TestRunner.php",
			} {
				if strings.Contains(
					phpServiceFileContent(t, stage.StaticFiles, ".dockerignore"), forbidden,
				) {
					t.Fatalf("focused PHP container projection retained application-wide input %s", forbidden)
				}
			}
		})
	}
}

func phpTaskStaticProjectionFixture(
	t *testing.T,
	responseMedia assemblyline.ApplicationServiceEndpointMedia,
) directCodingProgram {
	t.Helper()
	surface := assemblyline.ApplicationSurfaceBrowser
	if responseMedia == assemblyline.ApplicationServiceEndpointJSON {
		surface = assemblyline.ApplicationSurfaceService
	}
	specification, workload, target, coverage, endpoints := phpServiceStackFixtureForSurface(t, surface)
	state := testRequestLocalServiceStatePlan(workload)
	blueprint, staticFiles, err := compileGenericPHPServiceBlueprint(
		"php-service", specification, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
		state,
		endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	return directCodingProgram{
		StackID: genericPHPServiceAdapter, VersionProfileID: phpServiceVersionProfileV1,
		Workload: workload, Coverage: coverage,
		ServiceState:     state,
		ServiceEndpoints: endpoints, Source: blueprint, StaticFiles: staticFiles,
	}
}

func phpDockerfileContextCopyInputs(t *testing.T, dockerfile string) []string {
	t.Helper()
	inputs := make(map[string]struct{})
	for _, line := range strings.Split(dockerfile, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "COPY" || strings.HasPrefix(fields[1], "--from=") {
			continue
		}
		for _, input := range fields[1 : len(fields)-1] {
			inputs[input] = struct{}{}
		}
	}
	result := make([]string, 0, len(inputs))
	for input := range inputs {
		result = append(result, input)
	}
	sort.Strings(result)
	return result
}
