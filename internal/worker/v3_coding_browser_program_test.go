package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/specialists"
)

func TestGenericBrowserProgramUsesCompleteCodeOwnedVerification(t *testing.T) {
	t.Parallel()

	program := stubGenericBrowserProgram(t)
	commands, err := directCodingProgramVerificationCommands(genericBrowserSpecification(), program)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(commands))
	for _, command := range commands {
		if err := validateV3Command(command.Name, command.Args); err != nil {
			t.Fatalf("code-owned command %s %v is rejected: %v", command.Name, command.Args, err)
		}
		got = append(got, directCodingCommandLabel(command))
	}
	want := []string{
		"npm install --ignore-scripts --no-audit --no-fund --package-lock=false",
		"npm test", "npm run typecheck", "npm run build",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("commands=%v want=%v", got, want)
	}
}

func TestGenericBrowserProgramPinsItsSupportedToolchain(t *testing.T) {
	t.Parallel()

	files := typeScriptBrowserStaticFiles("unseen", "Application", genericBrowserStylesSource())
	var packageSource string
	for _, file := range files {
		if file.Path == "package.json" {
			packageSource = file.Content
		}
	}
	if packageSource == "" {
		t.Fatal("code-owned package.json is missing")
	}
	var manifest typeScriptPackageManifest
	if err := json.Unmarshal([]byte(packageSource), &manifest); err != nil {
		t.Fatal(err)
	}
	if got := manifest.DevDependencies["jsdom"]; got != "26.1.0" {
		t.Fatalf("jsdom=%q want Node 20-compatible 26.1.0", got)
	}
}

func TestGenericBrowserRuntimeContainsNoWorkloadDomainService(t *testing.T) {
	t.Parallel()

	api := genericBrowserRuntimeAPI(genericBrowserSpecification().Requirements)
	source := genericBrowserRuntimeSource(genericBrowserSpecification().Requirements)
	for _, forbidden := range []string{"ApplicationRuntime", "FeatureRuntime"} {
		if strings.Contains(api, forbidden) {
			t.Fatalf("model API exposed internal runtime %q:\n%s", forbidden, api)
		}
	}
	for _, forbidden := range []string{
		"AudioContext", "playTone", "useGlobalKeyboard", "brush", "sequencer", "character controller",
	} {
		if strings.Contains(api, forbidden) || strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Fatalf("workload-domain service %q remains in the generic runtime", forbidden)
		}
	}
	for _, required := range []string{
		"Unknown application capability", "Feature state key must start", "runtime.application.publish", "role=\"status\"", "role=\"alert\"",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("runtime source omitted explicit code-owned behavior %q", required)
		}
	}
}

func TestGenericBrowserStaticFilesContainNoWorkloadTestBackend(t *testing.T) {
	t.Parallel()

	files := typeScriptBrowserStaticFiles("unseen", "Application", genericBrowserStylesSource())
	contents := make(map[string]string, len(files))
	for _, file := range files {
		contents[file.Path] = file.Content
	}
	if _, exists := contents["src/test/browser.ts"]; exists {
		t.Fatal("generic browser project emitted a workload-specific browser backend")
	}
	if strings.Contains(contents["vite.config.ts"], "setupFiles") {
		t.Fatalf("Vitest loads an undeclared workload backend:\n%s", contents["vite.config.ts"])
	}
	for path, source := range contents {
		for _, forbidden := range []string{"TestAudioContext", "playTone", "AudioContext"} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("static file %s contains workload backend %q", path, forbidden)
			}
		}
	}
}

func TestGenericBrowserWorkspaceRejectsUndeclaredCompetingSource(t *testing.T) {
	t.Parallel()

	program := stubGenericBrowserProgram(t)
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
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
	diagnostic, err := directCodingProgramWorkspaceDiagnostic(root, program)
	if err != nil || diagnostic != nil {
		t.Fatalf("authoritative workspace diagnostic=%#v err=%v", diagnostic, err)
	}
	inspectionRoot := filepath.Join(root, ".omni", "runs", "481")
	if err := os.MkdirAll(inspectionRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inspectionRoot, "projection.ts"), []byte("derived inspection only\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	diagnostic, err = directCodingProgramWorkspaceDiagnostic(root, program)
	if err != nil || diagnostic != nil {
		t.Fatalf("internal task-state projection diagnostic=%#v err=%v", diagnostic, err)
	}
	legacy := filepath.Join(root, "src", "legacy.jsx")
	if err := os.WriteFile(legacy, []byte("export const Legacy = () => null;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	diagnostic, err = directCodingProgramWorkspaceDiagnostic(root, program)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic == nil || diagnostic.TargetPath != "src/legacy.jsx" || !strings.Contains(diagnostic.Detail, "undeclared") {
		t.Fatalf("competing-source diagnostic=%#v", diagnostic)
	}
}

func TestGenericBrowserSourceValidationAcceptsOnlyInMemoryAuthority(t *testing.T) {
	t.Parallel()

	specification := genericBrowserSpecification()
	program := stubGenericBrowserProgram(t)
	session := directCodingSession{program: &program, specification: &specification}
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range assembly.Files {
		if err := session.validateProgramSource(file.Path, file.Content); err != nil {
			t.Fatalf("validate authoritative %s: %v", file.Path, err)
		}
	}
	if err := session.validateProgramSource("src/App.tsx", "export function App() { return null; }"); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("tampered generated source error=%v", err)
	}
	if err := session.validateProgramSource("src/legacy.tsx", "export function Legacy() { return null; }"); err == nil || !strings.Contains(err.Error(), "undeclared") {
		t.Fatalf("undeclared source error=%v", err)
	}
}

func TestGenericBrowserProgramResolvesProtectedOpaqueArtifactsInCode(t *testing.T) {
	t.Parallel()

	specification := genericBrowserSpecification()
	specification.Artifacts = []assemblyline.ArtifactDirective{{
		Token: "ARTIFACT_1", Disposition: assemblyline.ArtifactProtect,
	}}
	program, err := compileDirectCodingProgram("unseen", specification, []assemblyline.ArtifactIdentity{{
		Token: "ARTIFACT_1", Value: "REQUEST.md",
	}}, genericBrowserSkillBindings(specification), genericBrowserCapabilityBindings(specification))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(program.ProtectedPaths, []string{"REQUEST.md"}) {
		t.Fatalf("protected=%v", program.ProtectedPaths)
	}
}

func TestGenericBrowserCodeOwnedAssemblyPassesNodeStage(t *testing.T) {
	if os.Getenv("OMNIDEX_NODE_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_NODE_INTEGRATION=1 to install the pinned browser toolchain")
	}
	program := stubGenericBrowserProgram(t)
	root := t.TempDir()
	if err := writeDirectCodingTypeScriptStage(root, program); err != nil {
		t.Fatal(err)
	}
	if output, err := runDirectCodingStageCommand(
		context.Background(), root, directCodingTypeScriptInstallTimeout, "npm", directCodingNPMInstallArgs()...,
	); err != nil {
		t.Fatalf("install pinned toolchain: %v\n%s", err, output)
	}
	diagnostic, err := verifyDirectCodingTypeScriptStage(context.Background(), root, program)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic != nil {
		t.Fatalf("generic assembly failed at block %s:\n%s", diagnostic.BlockID, diagnostic.Message)
	}
}

func stubGenericBrowserProgram(t *testing.T) directCodingProgram {
	t.Helper()
	specification := genericBrowserSpecification()
	program, err := compileDirectCodingProgram(
		"unseen", specification, nil, genericBrowserSkillBindings(specification),
		genericBrowserCapabilityBindings(specification),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, document := range program.TypeScript.Documents {
		for _, block := range document.Blocks {
			if block.Generated() {
				if strings.HasPrefix(block.ID, "feature.") && !strings.HasPrefix(block.ID, "feature.wrapper.") {
					program.Generated[block.ID] = block.Signature + fmt.Sprintf(
						` { return <button onClick={() => actions.set('ready', true)}>Working capability %s {String(state.ready ?? '')}</button>; }`,
						strings.TrimPrefix(block.ID, "feature."),
					)
					continue
				}
				sequence := strings.TrimPrefix(block.ID, "acceptance.")
				program.Generated[block.ID] = block.Signature + fmt.Sprintf(
					` { render(<Feature%s runtime={createFeatureRuntime(createApplicationRuntime(), 'capability_%s')} />); expect(screen.getByText(/Working capability/)).not.toBeNull(); }`,
					sequence, sequence,
				)
			}
		}
	}
	return program
}

func genericBrowserSpecification() assemblyline.ApplicationSpecification {
	return assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: "catalog browser",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "filter the catalog"},
			{ID: "requirement_002", SourceQuote: "remember my selection"},
		},
	}
}

func genericBrowserSkillBindings(
	specification assemblyline.ApplicationSpecification,
) map[string]directCodingSkillBinding {
	bindings := make(map[string]directCodingSkillBinding, len(specification.Requirements))
	for _, requirement := range specification.Requirements {
		bindings[requirement.ID] = directCodingSkillBinding{
			RequirementID: requirement.ID,
			Procedure:     "Implement the stated local behavior with accessible controls and an observable state transition.",
			Version:       specialists.SkillVersion{Status: specialists.SkillStatusActive},
		}
	}
	return bindings
}

func genericBrowserCapabilityBindings(
	specification assemblyline.ApplicationSpecification,
) directCodingCapabilityGraph {
	graph := make(directCodingCapabilityGraph, len(specification.Requirements))
	for _, requirement := range specification.Requirements {
		graph[requirement.ID] = nil
	}
	if len(specification.Requirements) > 1 {
		graph[specification.Requirements[1].ID] = []directCodingCapabilityBinding{{
			RequirementID: specification.Requirements[0].ID,
			CapabilityID:  genericBrowserCapabilityID(1),
			Purpose:       specification.Requirements[0].SourceQuote,
		}}
	}
	return graph
}
