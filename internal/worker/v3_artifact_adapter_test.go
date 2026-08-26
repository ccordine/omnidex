package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestArtifactAdapterRegistryProvidesTreeStackAndClassifiesLeaves(t *testing.T) {
	if err := validateDirectCodingArtifactRegistries(); err != nil {
		t.Fatal(err)
	}
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	context, err := directCodingTreeTechnicalContext(stack)
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
		{"app/Http/Controllers/PatientController.php", "php"},
		{"database/migrations/001_service_state.sql", "postgresql_migration"},
		{"resources/js/controllers/patient_filter_controller.js", "javascript"},
		{"resources/js/components/patient_filter.jsx", "javascript"},
		{"scripts/check.mjs", "javascript"},
		{"resources/css/app.css", "css_tailwind"},
		{"docker/nginx/default.conf", "nginx"},
		{"Dockerfile", "dockerfile"},
		{"docker-compose.yml", "dockerfile"},
		{"src/main/java/Counter.java", "java"},
		{"src/main.rs", "rust"},
		{"cmd/server/main.go", "go"},
		{"go.mod", "go_module"},
		{"package.json", "structured_json"},
		{"deploy/values.yaml", "structured_yaml"},
		{".env.example", "environment_example"},
		{".gitignore", "plain_text"},
		{".dockerignore", "plain_text"},
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
	if _, _, err := directCodingArtifactAdapterForPath(
		"resources/views/patients/index.blade.php",
	); err == nil || !strings.Contains(err.Error(), "no registered adapter") {
		t.Fatalf("unsupported Blade artifact did not fail loudly: %v", err)
	}
	for _, unsafe := range []string{
		"database/migrations/../outside.sql", "/database/migrations/001.sql",
	} {
		if _, _, err := directCodingArtifactAdapterForPath(unsafe); err == nil {
			t.Fatalf("PostgreSQL migration adapter accepted unsafe path %q", unsafe)
		}
	}
}

func TestCSSAdapterClaimsOnlyItsExecutableStructuralValidation(t *testing.T) {
	adapter, err := directCodingArtifactAdapterByID("css_tailwind")
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Validation.Kind != directCodingArtifactStructural || adapter.Validation.Execute == nil {
		t.Fatalf("CSS adapter validation=%+v, want executable structural validation", adapter.Validation)
	}
	if err := validateDirectCodingArtifactSource(
		adapter, "resources/styles.css", []byte("main { color red; }\n"),
	); err != nil {
		t.Fatalf("structurally balanced CSS was treated as syntax-parsed: %v", err)
	}
}

func TestArtifactAdaptersExecuteSourceValidationAcrossUnrelatedClasses(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		path    string
		valid   string
		invalid string
	}{
		{name: "TypeScript React", path: "src/View.tsx", valid: "export function View(): JSX.Element { return <main />; }", invalid: "export function View( {"},
		{name: "TypeScript", path: "src/value.ts", valid: "export function value(): number { return 1; }", invalid: "export function value(: number {"},
		{name: "Go", path: "cmd/service/main.go", valid: "package main\nfunc main() {}\n", invalid: "package main\nfunc main( {\n"},
		{name: "Go module", path: "go.mod", valid: "module example.invalid/application\n\ngo 1.25\n", invalid: "module example.invalid/application\n"},
		{name: "PHP", path: "app/Value.php", valid: "<?php\nfunction value(): int { return 1; }\n", invalid: "<?php\nfunction value( {\n"},
		{name: "JavaScript", path: "resources/js/value.js", valid: "export function value() { return 1; }\n", invalid: "export function value( {\n"},
		{name: "CSS", path: "resources/css/app.css", valid: "@tailwind utilities;\nmain { display: grid; }\n", invalid: "main { display: grid;\n"},
		{name: "HTML", path: "public/index.html", valid: "<!doctype html><html><body><main></main></body></html>\n", invalid: "<html><body><main class=></main></body></html>"},
		{name: "Java", path: "src/main/java/Value.java", valid: "final class Value { int get() { return 1; } }\n", invalid: "final class { int get() { return 1; } }\n"},
		{name: "Rust", path: "src/main.rs", valid: "fn main() { println!(\"ready\"); }\n", invalid: "fn main( {\n"},
		{name: "NGINX", path: "docker/nginx/default.conf", valid: "server { listen 8080; location / { try_files $uri /index.html; } }\n", invalid: "server { listen 8080 }\n"},
		{name: "Dockerfile", path: "Dockerfile", valid: "FROM alpine:3.21\nRUN echo ready\n", invalid: "FROM\n"},
		{name: "Dockerfile unknown directive", path: "Dockerfile", valid: "FROM alpine:3.21\n", invalid: "BANANA value\n"},
		{name: "Compose", path: "docker-compose.yml", valid: "services:\n  app:\n    image: alpine:3.21\n", invalid: "services: [\n"},
		{name: "JSON", path: "package.json", valid: "{\"private\":true}\n", invalid: "{\"private\":true,}\n"},
		{name: "YAML", path: "deploy/values.yaml", valid: "replicas: 2\n", invalid: "replicas: [\n"},
		{name: "environment", path: ".env.example", valid: "APP_ENV=local\n", invalid: "1APP_ENV=local\n"},
		{name: "PostgreSQL migration", path: "database/migrations/001_values.sql", valid: "BEGIN;\nCREATE TABLE IF NOT EXISTS values_table (id BIGINT PRIMARY KEY);\nCOMMIT;\n", invalid: "DROP TABLE values_table;\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			adapter, _, err := directCodingArtifactAdapterForPath(testCase.path)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateDirectCodingArtifactSource(adapter, testCase.path, []byte(testCase.valid)); err != nil {
				t.Fatalf("valid source rejected: %v", err)
			}
			if err := validateDirectCodingArtifactSource(adapter, testCase.path, []byte(testCase.invalid)); err == nil {
				t.Fatal("invalid source was accepted")
			}
		})
	}
}

func TestTypeScriptBrowserStackSeparatesWorkloadLeavesFromAllEmittedArtifacts(t *testing.T) {
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := directCodingArtifactAdapterForTreePath(stack, "src/Feature.tsx"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := directCodingArtifactAdapterForProjectPath(stack, "package.json"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := directCodingArtifactAdapterForTreePath(stack, "package.json"); err == nil || !strings.Contains(err.Error(), "selected project stack") {
		t.Fatalf("tree accepted code-owned manifest leaf: %v", err)
	}
	locked, err := typeScriptBrowserStaticFiles(
		requireDirectCodingVersionProfile(t, typeScriptBrowserVersionProfileV1),
		"adapter-registry", "Adapter registry fixture", "main { display: block; }",
	)
	if err != nil {
		t.Fatal(err)
	}
	validFiles := make([]directCodingFileTask, 0, 7)
	for _, file := range locked {
		if file.Path == "package.json" || file.Path == "package-lock.json" || file.Path == "vite.config.ts" {
			validFiles = append(validFiles, file)
		}
	}
	validFiles = append(validFiles, []directCodingFileTask{
		{Path: "index.html", Content: `<div id="root"></div><script src="/src/main.tsx"></script>`},
		{Path: "src/main.tsx", Content: "import { App } from './App';\nimport './styles.css';\n"},
		{Path: "src/App.tsx", Content: "export function App(): JSX.Element { return <main />; }\n"},
		{Path: "src/styles.css", Content: typeScriptTailwindStylesSource("main { display: block; }")},
	}...)
	validAssembly := directCodingAssembly{Files: validFiles}
	if err := stack.ValidateAssembly(validAssembly); err != nil {
		t.Fatal(err)
	}
	invalidAssembly := directCodingAssembly{Files: []directCodingFileTask{{Path: "index.html", Content: `<main></main>`}}}
	if err := stack.ValidateAssembly(invalidAssembly); err == nil {
		t.Fatal("stack accepted an entrypoint disconnected from its code-owned runtime")
	}
	missingTarget := directCodingAssembly{Files: make([]directCodingFileTask, 0, len(validAssembly.Files)-1)}
	for _, file := range validAssembly.Files {
		if file.Path != "src/App.tsx" {
			missingTarget.Files = append(missingTarget.Files, file)
		}
	}
	if err := stack.ValidateAssembly(missingTarget); err == nil || !strings.Contains(err.Error(), "absent artifact src/App.tsx") {
		t.Fatalf("cross-artifact relation error=%v", err)
	}
	commands, err := stack.VerificationCommands(directCodingProgram{StackID: stack.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 4 || commands[0].Name != "npm" || commands[3].Name != "npm" {
		t.Fatalf("TypeScript stack verification commands=%+v", commands)
	}
	java, err := directCodingArtifactAdapterByID("java")
	if err != nil {
		t.Fatal(err)
	}
	if java.Validation.Kind != directCodingArtifactParse || java.Validation.Execute == nil {
		t.Fatalf("Java leaf validation=%+v, want executable parser", java.Validation)
	}
}
