package worker

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProgramWriteGateUsesSelectedArtifactValidator(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		path    string
		content string
		wantErr string
	}{
		{name: "valid JSON", path: "package.json", content: "{\"private\":true}\n"},
		{name: "invalid JSON", path: "package.json", content: "{\"private\":true,}\n", wantErr: "invalid JSON"},
		{name: "invalid TypeScript", path: "src/main.tsx", content: "export function main( {\n", wantErr: "typescript_react adapter rejected"},
		{name: "disconnected entrypoint", path: "index.html", content: "<!doctype html><main></main>\n", wantErr: "lacks its required root"},
		{name: "adapter outside stack", path: "main.go", content: "package main\nfunc main() {}\n", wantErr: "not registered in project stack"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			program := typeScriptBrowserTailwindFixture(t)
			if testCase.name == "valid JSON" {
				testCase.content = directCodingTestFileContent(t, program.StaticFiles, testCase.path)
			}
			replaced := false
			for index := range program.StaticFiles {
				if program.StaticFiles[index].Path == testCase.path {
					program.StaticFiles[index].Content = testCase.content
					replaced = true
				}
			}
			if !replaced {
				program.StaticFiles = append(program.StaticFiles, directCodingFileTask{
					Path: testCase.path, Content: testCase.content,
				})
			}
			err := validateDirectCodingProgramSource(testCase.path, testCase.content, program)
			if testCase.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("error=%v want substring %q", err, testCase.wantErr)
			}
		})
	}
}

func TestProgramWriteGateRejectsBytesOutsideInMemoryAuthorityBeforeParsing(t *testing.T) {
	program := directCodingProgram{
		StackID: genericTypeScriptBrowserAdapter, VersionProfileID: typeScriptBrowserVersionProfileV1,
		StaticFiles: []directCodingFileTask{{
			Path: "package.json", Content: "{\"private\":true}\n",
		}},
		Generated: map[string]string{},
	}
	err := validateDirectCodingProgramSource("package.json", "{\"private\":false}\n", program)
	if err == nil || !strings.Contains(err.Error(), "differs from its parser-validated in-memory authority") {
		t.Fatalf("error=%v", err)
	}
}

func TestCompleteAssemblySieveValidatesEveryArtifactBeforeMutation(t *testing.T) {
	locked, err := typeScriptBrowserStaticFiles(
		requireDirectCodingVersionProfile(t, typeScriptBrowserVersionProfileV1),
		"assembly-sieve", "Assembly sieve", "main { display: grid; }",
	)
	if err != nil {
		t.Fatal(err)
	}
	staticFiles := make([]directCodingFileTask, 0, len(locked)+2)
	for _, file := range locked {
		switch file.Path {
		case "package.json", "package-lock.json", "index.html", "tsconfig.json", "vite.config.ts", "src/styles.css", ".gitignore":
			staticFiles = append(staticFiles, file)
		}
	}
	staticFiles = append(staticFiles,
		directCodingFileTask{Path: "src/main.tsx", Content: "import { App } from './App';\nimport './styles.css';\nvoid App;\n"},
		directCodingFileTask{Path: "src/App.tsx", Content: "export function App(): JSX.Element { return <main />; }\n"},
	)
	program := directCodingProgram{
		StackID: genericTypeScriptBrowserAdapter, VersionProfileID: typeScriptBrowserVersionProfileV1,
		StaticFiles: staticFiles,
		Generated:   map[string]string{},
	}
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingAssemblySources(program, assembly); err != nil {
		t.Fatal(err)
	}
	mutatedStyles := false
	for index := range assembly.Files {
		if assembly.Files[index].Path == "src/styles.css" {
			assembly.Files[index].Content = "main { display: grid;\n"
			mutatedStyles = true
		}
	}
	if !mutatedStyles {
		t.Fatal("assembly sieve fixture lacks src/styles.css")
	}
	if err := validateDirectCodingAssemblySources(program, assembly); err == nil || !strings.Contains(err.Error(), "unclosed block") {
		t.Fatalf("error=%v", err)
	}
}

func TestProgramWriteGateContainsNoCentralLanguageSwitch(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve validation test source")
	}
	for _, testCase := range []struct {
		file      string
		forbidden []string
	}{
		{file: "v3_coding_program_validation.go", forbidden: []string{"filepath.Ext", `case ".ts"`, "ValidateTypeScriptSource", "json.Valid"}},
		{file: "v3_coding_program_workspace.go", forbidden: []string{"filepath.Ext", `case ".ts"`, `path == "package.json"`, `path == "tsconfig.json"`}},
	} {
		source, err := os.ReadFile(filepath.Join(filepath.Dir(current), testCase.file))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range testCase.forbidden {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("central artifact gate %s regained language-specific authority %q", testCase.file, forbidden)
			}
		}
	}
}

func TestUnexpectedSourceDetectionUsesTheSelectedStackRegistry(t *testing.T) {
	program := directCodingProgram{StackID: genericTypeScriptBrowserAdapter}
	for _, artifactPath := range []string{
		"package.json", "src/main.tsx", "src/styles.css", "public/extra.html",
	} {
		recognized, err := directCodingProgramSourcePath(artifactPath, program)
		if err != nil {
			t.Fatal(err)
		}
		if !recognized {
			t.Fatalf("stack-owned artifact %q escaped unexpected-source detection", artifactPath)
		}
	}
	for _, artifactPath := range []string{"README.md", "main.go", "scripts/check.mjs", "Dockerfile", "docker/nginx/default.conf"} {
		recognized, err := directCodingProgramSourcePath(artifactPath, program)
		if err != nil {
			t.Fatal(err)
		}
		if recognized {
			t.Fatalf("non-member artifact %q was claimed by the TypeScript stack", artifactPath)
		}
	}
}
