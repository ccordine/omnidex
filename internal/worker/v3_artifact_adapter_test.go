package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestArtifactAdapterRegistryProvidesTreeStackAndClassifiesLeaves(t *testing.T) {
	stack, context, err := directCodingTreeTechnicalContext(assemblyline.ApplicationSurfaceBrowser, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(context, "TypeScript React") || !strings.Contains(context, ".test.tsx") {
		t.Fatalf("technical context=%q", context)
	}
	adapter, kind, err := directCodingArtifactAdapterForTreePath(stack, "tests/counter.test.tsx")
	if err != nil {
		t.Fatal(err)
	}
	if adapter.ID != "typescript_react" || kind != assemblyline.TargetArtifactVerification {
		t.Fatalf("adapter=%+v kind=%q", adapter, kind)
	}
	java, err := directCodingArtifactAdapterByID("java")
	if err != nil {
		t.Fatal(err)
	}
	if kind, recognized := java.Recognize("src/main/java/Counter.java"); !recognized || kind != assemblyline.TargetArtifactImplementation {
		t.Fatalf("java recognition kind=%q recognized=%t", kind, recognized)
	}
	if _, _, err := directCodingArtifactAdapterForTreePath(stack, "src/main/java/Counter.java"); err == nil || !strings.Contains(err.Error(), "selected project stack") {
		t.Fatalf("selected stack error=%v", err)
	}
}

func TestArtifactAdaptersRecognizeNamedArtifactClassesByPath(t *testing.T) {
	for _, testCase := range []struct {
		path string
		id   string
	}{
		{"app/Http/Controllers/PatientController.php", "php_laravel"},
		{"resources/views/patients/index.blade.php", "blade_html"},
		{"resources/js/controllers/patient_filter_controller.js", "javascript_stimulus"},
		{"resources/css/app.css", "css_tailwind"},
		{"docker/nginx/default.conf", "nginx"},
		{"Dockerfile", "dockerfile"},
		{"docker-compose.yml", "dockerfile"},
		{"src/main/java/Counter.java", "java"},
		{"cmd/server/main.go", "go"},
		{"package.json", "structured_json"},
		{"deploy/values.yaml", "structured_yaml"},
		{".env.example", "environment_example"},
	} {
		t.Run(testCase.id, func(t *testing.T) {
			adapter, _, err := directCodingArtifactAdapterForPath(testCase.path)
			if err != nil {
				t.Fatal(err)
			}
			if adapter.ID != testCase.id {
				t.Fatalf("adapter=%q want=%q", adapter.ID, testCase.id)
			}
		})
	}
}
