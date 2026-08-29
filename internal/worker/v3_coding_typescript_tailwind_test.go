package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTypeScriptBrowserAssemblyOwnsTailwindViteToolchain(t *testing.T) {
	program := typeScriptBrowserTailwindFixture(t)
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingAssemblySources(program, assembly); err != nil {
		t.Fatal(err)
	}
	files := make(map[string]string, len(assembly.Files))
	for _, file := range assembly.Files {
		files[file.Path] = file.Content
	}
	for _, required := range []string{
		"import tailwindcss from '@tailwindcss/vite';",
		"plugins: [react(), tailwindcss()]",
	} {
		if !strings.Contains(files["vite.config.ts"], required) {
			t.Fatalf("code-owned Vite config omits %q", required)
		}
	}
	if !strings.HasPrefix(files["src/styles.css"], typeScriptBrowserTailwindImport+"\n") {
		t.Fatal("code-owned stylesheet omits the Tailwind CSS import")
	}
	if !strings.Contains(files["src/App.tsx"], `className="isolate"`) {
		t.Fatal("fixture omits the utility whose emitted CSS is verified")
	}
}

func TestTypeScriptBrowserTailwindUsageDoesNotExposeToolchainAuthority(t *testing.T) {
	contract := genericBrowserFeatureContract("Render one bounded view.", nil)
	if !strings.Contains(contract, "Tailwind CSS utility classes are available in className") ||
		!strings.Contains(contract, "Use only complete static non-arbitrary utilities") ||
		!strings.Contains(contract, "unknown or custom classes are unavailable") {
		t.Fatal("browser fragment contract omits its usable styling capability")
	}
	for _, forbidden := range []string{"@tailwindcss/vite", "vite.config", "package.json", "npm ", "src/styles.css"} {
		if strings.Contains(contract, forbidden) {
			t.Fatalf("browser fragment contract exposes code-owned toolchain authority %q", forbidden)
		}
	}
}

func TestBrowserFeatureContractContainsBehaviorWithoutFrameworkMetaFraming(t *testing.T) {
	t.Parallel()
	contract := genericBrowserFeatureContract("Show the current inventory level.", nil)
	if !strings.Contains(contract, "Show the current inventory level.") ||
		!strings.Contains(contract, "declared inputs") {
		t.Fatalf("browser behavior contract omitted semantic authority: %s", contract)
	}
	for _, forbidden := range []string{
		"task-neutral", "code-owned boundary", "block", "workload-specific", "local contract",
	} {
		if strings.Contains(contract, forbidden) {
			t.Fatalf("browser behavior contract exposed framework meta-framing %q: %s", forbidden, contract)
		}
	}
}

func TestTypeScriptBrowserAssemblyRejectsTailwindAuthorityDrift(t *testing.T) {
	for _, testCase := range []struct {
		name, path, old, replacement, want string
	}{
		{
			name: "Vite plugin removed", path: "vite.config.ts",
			old: ", tailwindcss()", replacement: "", want: "code-owned Tailwind authority",
		},
		{
			name: "CSS import removed", path: "src/styles.css",
			old: typeScriptBrowserTailwindImport + "\n\n", replacement: "", want: "Tailwind import",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			program := typeScriptBrowserTailwindFixture(t)
			for index := range program.StaticFiles {
				file := &program.StaticFiles[index]
				if file.Path == testCase.path {
					file.Content = strings.Replace(file.Content, testCase.old, testCase.replacement, 1)
				}
			}
			assembly, err := directCodingAssemblyFromProgram(program)
			if err != nil {
				t.Fatal(err)
			}
			err = validateDirectCodingAssemblySources(program, assembly)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error=%v want substring %q", err, testCase.want)
			}
		})
	}
}

func TestTypeScriptBrowserTailwindBuildEmitsUsedUtility(t *testing.T) {
	if os.Getenv("OMNIDEX_NODE_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_NODE_INTEGRATION=1 to install and build the pinned browser toolchain")
	}
	program := typeScriptBrowserTailwindFixture(t)
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingAssemblySources(program, assembly); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, file := range assembly.Files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{directCodingNPMInstallArgs(), {"run", "build"}} {
		output, err := runDirectCodingStageCommand(
			context.Background(), root, directCodingTypeScriptInstallTimeout, "npm", args...,
		)
		if err != nil {
			t.Fatalf("npm %s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
	assets, err := filepath.Glob(filepath.Join(root, "dist", "assets", "*.css"))
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 {
		t.Fatalf("Vite emitted %d CSS assets, want exactly one", len(assets))
	}
	css, err := os.ReadFile(assets[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".isolate{isolation:isolate}") {
		t.Fatalf("built CSS lacks the used Tailwind utility rule: %s", trimForBudget(string(css), 2_000))
	}
}

func typeScriptBrowserTailwindFixture(t *testing.T) directCodingProgram {
	t.Helper()
	files, err := typeScriptBrowserStaticFiles(
		requireDirectCodingVersionProfile(t, typeScriptBrowserVersionProfileV1),
		"tailwind-fixture", "Tailwind fixture", genericBrowserStylesSource(),
	)
	if err != nil {
		t.Fatal(err)
	}
	files = append(files,
		directCodingFileTask{
			Path: "src/App.tsx",
			Content: `import type { ReactElement } from 'react';

export function App(): ReactElement {
  return <main className="isolate">Tailwind fixture</main>;
}
`,
		},
		directCodingFileTask{Path: "src/main.tsx", Content: typeScriptWebMainSource()},
	)
	return directCodingProgram{
		StackID: genericTypeScriptBrowserAdapter, VersionProfileID: typeScriptBrowserVersionProfileV1,
		StaticFiles: files,
		Generated:   map[string]string{},
	}
}
