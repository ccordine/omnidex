package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPHPHTMLRendererIsOneEscapedServerRepresentation(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language:  "php",
		Dialect:   "PHP >=8.2,<9",
		Signature: "function renderFeature101HTML(TaskResult $result): string",
		Behavior:  "Render one representation.",
		PermittedSymbols: []string{
			phpServiceTaskResultAPI(), phpServiceHTMLRuntimeAPI(),
		},
	}
	if _, err := validateDirectCodingPHPFragment(input, phpServiceHTMLRendererFixture()); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"raw result": `function renderFeature101HTML(TaskResult $result): string {
    return RuntimeHtml::document('Title', '<main class="md:p-4"><h1>Title</h1>' . $result->output . '</main>');
}`,
		"static result": `function renderFeature101HTML(TaskResult $result): string {
    return RuntimeHtml::document('Title', '<main class="md:p-4"><h1>Title</h1><p>constant</p></main>');
}`,
		"split script": `function renderFeature101HTML(TaskResult $result): string {
    return RuntimeHtml::document('Title', '<main class="md:p-4"><h1>Title</h1><scr' . 'ipt>' . RuntimeHtml::escape($result->output) . '</script></main>');
}`,
		"event handler": `function renderFeature101HTML(TaskResult $result): string {
    return RuntimeHtml::document('Title', '<main class="md:p-4" onclick="run()"><h1>Title</h1>' . RuntimeHtml::escape($result->output) . '</main>');
}`,
		"no responsive utility": `function renderFeature101HTML(TaskResult $result): string {
    return RuntimeHtml::document('Title', '<main class="p-4"><h1>Title</h1>' . RuntimeHtml::escape($result->output) . '</main>');
}`,
		"branching renderer": `function renderFeature101HTML(TaskResult $result): string {
    if ($result->output === '') { return RuntimeHtml::document('Empty', '<main class="md:p-4"><h1>Empty</h1></main>'); }
    return RuntimeHtml::document('Title', '<main class="md:p-4"><h1>Title</h1>' . RuntimeHtml::escape($result->output) . '</main>');
}`,
		"wrong field encoder": `function renderFeature101HTML(TaskResult $result): string {
    return RuntimeHtml::document('Title', '<main class="md:p-4"><h1>Title</h1>' . RuntimeHtml::state($result->output) . '</main>');
}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateDirectCodingPHPFragment(input, source); err == nil {
				t.Fatalf("accepted unsafe HTML renderer:\n%s", source)
			}
		})
	}
}

func TestPHPHTMLRendererTraversesRecordsAndUsesOpaqueRouteCapability(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language:  "php",
		Dialect:   "PHP >=8.2,<9",
		Signature: "function renderFeature101HTML(TaskResult $result): string",
		Behavior:  "Render one record collection with its registered mutation interaction.",
		PermittedSymbols: []string{
			phpServiceTaskResultAPI(), phpServiceHTMLRuntimeAPI(),
			"function routeFeature102(string $record_id): RuntimeRoute",
		},
	}
	source := `function renderFeature101HTML(TaskResult $result): string {
    $items = '';
    foreach (RuntimeHtml::records($result->state, 'records') as $record) {
        $items .= '<article class="rounded border p-4"><h2>' .
            RuntimeHtml::escape(RuntimeHtml::field($record, 'title')) . '</h2>' .
            RuntimeHtml::formOpen(routeFeature102(RuntimeHtml::field($record, 'id'))) .
            '<button type="submit">Archive</button>' . RuntimeHtml::formClose() . '</article>';
    }
    return RuntimeHtml::document(
        'Records',
        '<main class="min-h-screen p-4 md:p-8"><h1>Records</h1>' . $items . '</main>',
    );
}`
	if _, err := validateDirectCodingPHPFragment(input, source); err != nil {
		t.Fatal(err)
	}
	unsafe := strings.Replace(
		source,
		"RuntimeHtml::escape(RuntimeHtml::field($record, 'title'))",
		"RuntimeHtml::field($record, 'title')", 1,
	)
	if _, err := validateDirectCodingPHPFragment(input, unsafe); err == nil {
		t.Fatal("accepted an unescaped record field")
	}
}

func TestPHPRouteCapabilityOwnsPathMethodAndParameterEncoding(t *testing.T) {
	t.Parallel()
	binding := phpServiceFeatureBinding{
		TaskID: "task_002", FeatureNumber: "102", RouteBlockID: "runtime.route.002",
		RouteName: "routeFeature102", HasEndpoint: true,
		Endpoint: assemblyline.ApplicationServiceEndpointContract{
			Schema:        assemblyline.ApplicationServiceEndpointContractSchemaV1,
			Exposure:      assemblyline.ApplicationServiceEndpointPublic,
			Method:        assemblyline.ApplicationServiceEndpointPATCH,
			RouteTemplate: "/records/{record_id}",
			RequestMedia:  assemblyline.ApplicationServiceEndpointForm,
			ResponseMedia: assemblyline.ApplicationServiceEndpointHTML,
			SuccessStatus: 200,
		},
	}
	signature, source, err := phpServiceRouteFunction(binding)
	if err != nil {
		t.Fatal(err)
	}
	if signature != "function routeFeature102(string $record_id): RuntimeRoute" ||
		!strings.Contains(source, "new RuntimeRoute('/' . 'records' . '/' . rawurlencode($record_id), 'PATCH')") ||
		strings.Contains(signature, "/records") {
		t.Fatalf("route signature/source differs:\n%s\n%s", signature, source)
	}
}

func TestPHPHTMLRepresentationIsASeparateGeneratedTaskLeaf(t *testing.T) {
	t.Parallel()
	specification, workload, target, coverage, endpoints := phpServiceStackFixture(t)
	blueprint, _, err := compileGenericPHPServiceBlueprint(
		"php-service", specification, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
		testRequestLocalServiceStatePlan(workload), endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	representation := assemblyline.SourceBlock{}
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.Role == assemblyline.SourceBlockTaskRepresentation {
				representation = block
			}
		}
	}
	if representation.ID != "representation.html.001" ||
		representation.Signature != "function renderFeature101HTML(TaskResult $result): string" ||
		!representation.Generated() || representation.TaskID != workload.Tasks[0].ID {
		t.Fatalf("HTML representation leaf=%+v", representation)
	}
}

func TestPHPJSONServiceOmitsHTMLLeafAndTailwindToolchain(t *testing.T) {
	t.Parallel()
	specification, workload, target, coverage, endpoints := phpServiceStackFixtureForSurface(
		t, assemblyline.ApplicationSurfaceService,
	)
	blueprint, files, err := compileGenericPHPServiceBlueprint(
		"php-service", specification, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
		testRequestLocalServiceStatePlan(workload), endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.Role == assemblyline.SourceBlockTaskRepresentation || block.ID == "runtime.html" {
				t.Fatalf("JSON-only PHP service retained HTML block %s", block.ID)
			}
		}
	}
	for _, forbidden := range []string{"package.json", "package-lock.json", "resources/styles.css"} {
		for _, file := range files {
			if file.Path == forbidden {
				t.Fatalf("JSON-only PHP service retained %s", forbidden)
			}
		}
	}
	dockerfile := phpServiceFileContent(t, files, "Dockerfile")
	for _, forbidden := range []string{phpServiceNodeImage, "npm ", "tailwind", "public/assets/app.css"} {
		if strings.Contains(strings.ToLower(dockerfile), strings.ToLower(forbidden)) {
			t.Fatalf("JSON-only Dockerfile retained asset authority %s", forbidden)
		}
	}
}
