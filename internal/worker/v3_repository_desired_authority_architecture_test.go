package worker

import (
	"go/ast"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDesiredStateModelContractsExposeOnlySemanticLeaves(t *testing.T) {
	t.Parallel()
	contracts := []reflect.Type{
		reflect.TypeOf(assemblyline.RepositoryRequirementInterpretationInput{}),
		reflect.TypeOf(assemblyline.RepositoryRequirementInterpretation{}),
		reflect.TypeOf(assemblyline.ArtifactHandlingInput{}),
		reflect.TypeOf(assemblyline.ArtifactHandlingDecision{}),
		reflect.TypeOf(assemblyline.KnownArtifactTruthInput{}),
		reflect.TypeOf(assemblyline.KnownArtifactTruthDecision{}),
		reflect.TypeOf(assemblyline.DeclarationArtifactBoundaryInput{}),
		reflect.TypeOf(assemblyline.DeclarationArtifactBoundaryDecision{}),
		reflect.TypeOf(assemblyline.ArtifactCandidateEvidence{}),
		reflect.TypeOf(assemblyline.ArtifactCandidateSelectionInput{}),
		reflect.TypeOf(assemblyline.ArtifactCandidateSelectionDecision{}),
		reflect.TypeOf(assemblyline.FragmentGenerationInput{}),
		reflect.TypeOf(assemblyline.FragmentCorrectionInput{}),
	}
	forbiddenFields := stringSet(
		"path", "paths", "filepath", "filepaths", "filename", "filenames",
		"file", "files", "targetpath", "targetpaths", "operation", "operations",
		"action", "actions", "command", "commands", "shell", "patch", "patches",
		"content", "contents", "filecontent", "filecontents", "wholefile",
		"workspace", "tree", "tool", "tools", "arguments", "completion", "plan",
	)
	for _, contract := range contracts {
		for index := 0; index < contract.NumField(); index++ {
			field := contract.Field(index)
			for _, name := range []string{field.Name, strings.Split(field.Tag.Get("json"), ",")[0]} {
				if _, forbidden := forbiddenFields[normalizeAuthorityName(name)]; forbidden {
					t.Errorf("model contract %s exposes physical authority field %q", contract.Name(), name)
				}
			}
		}
	}
}

func TestDesiredStateModelSchemasContainNoMutationToolSurface(t *testing.T) {
	t.Parallel()
	repositoryRequest := "ARTIFACT_1 must no longer exist"
	repositoryContext, err := assemblyline.BootstrapApplicationContext(
		repositoryRequest, assemblyline.ApplicationWorkspaceExisting, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	must := func(job assemblyline.PortableJob, err error) assemblyline.PortableJob {
		if err != nil {
			t.Fatal(err)
		}
		return job
	}
	jobs := []assemblyline.PortableJob{
		must(assemblyline.NewRepositoryRequirementInterpretationJob(assemblyline.RepositoryRequirementInterpretationInput{
			UserRequest: repositoryRequest, Context: repositoryContext,
		})),
		must(assemblyline.NewArtifactHandlingJob(assemblyline.ArtifactHandlingInput{
			UserRequest: "ARTIFACT_1 must no longer exist", Token: "ARTIFACT_1",
		})),
		must(assemblyline.NewKnownArtifactTruthJob(assemblyline.KnownArtifactTruthInput{
			RequirementQuote: "One known semantic artifact must no longer exist",
		})),
		must(assemblyline.NewDeclarationArtifactBoundaryJob(assemblyline.DeclarationArtifactBoundaryInput{
			RequirementQuote: "func Added() string has an independent artifact boundary",
			GoSignature:      "func Added() string", DeclarationID: "DECLARATION_1",
		})),
		must(assemblyline.NewArtifactCandidateSelectionJob(assemblyline.ArtifactCandidateSelectionInput{
			RequirementQuote: "ARTIFACT_1 or ARTIFACT_2 must no longer be present",
			Candidates: []assemblyline.ArtifactCandidateEvidence{
				{CandidateID: "ARTIFACT_CANDIDATE_1", Declarations: []string{"func OldOne() int"}},
				{CandidateID: "ARTIFACT_CANDIDATE_2", Declarations: []string{"func OldTwo() int"}},
			},
		})),
		must(assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
			Language: "go", Dialect: "Go 1.24", Signature: "func Added() string", Behavior: "Return a stable semantic value.",
		})),
	}
	for _, job := range jobs {
		prompt, schema, err := assemblyline.RenderPortableJob(job)
		if err != nil {
			t.Fatalf("render %s: %v", job.Kind, err)
		}
		if strings.Contains(prompt, "omni_added_artifact.go") {
			t.Errorf("%s renderer exposed a code-derived target path", job.Kind)
		}
		assertNoPhysicalSchemaAuthority(t, string(job.Kind), schema)
	}
}

func TestForbiddenModelAuthorityIdentifierMatchesWholeConceptWords(t *testing.T) {
	t.Parallel()
	for _, identifier := range []string{
		"WriteFile", "SomeToolCallSchema", "ActionSchemaV1", "UniversalAgentRuntime",
	} {
		if !forbiddenModelAuthorityIdentifier(identifier) {
			t.Errorf("forbidden model authority identifier %q was not detected", identifier)
		}
	}
	for _, identifier := range []string{
		"RoleplayCanonExtractionSchemaV1", "DatabaseQueryIntentSchemaV1", "ApplicationContext",
	} {
		if forbiddenModelAuthorityIdentifier(identifier) {
			t.Errorf("semantic identifier %q was rejected by a cross-word substring match", identifier)
		}
	}
}

func TestDesiredStateModelSourcesHaveNoFilesystemOrUniversalAgentAuthority(t *testing.T) {
	t.Parallel()
	for _, source := range desiredModelBoundSources(t) {
		for _, imported := range source.imports {
			switch imported {
			case "os", "os/exec", "io/ioutil", "syscall",
				"github.com/gryph/omnidex/internal/omni",
				"github.com/gryph/omnidex/internal/repository/changeapply",
				"github.com/gryph/omnidex/internal/tools",
				"github.com/gryph/omnidex/internal/hostbridge",
				"github.com/gryph/omnidex/internal/agentconfig",
				"github.com/gryph/omnidex/internal/agentstream":
				t.Errorf("model-bound source %s imports forbidden authority %q", source.path, imported)
			}
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && forbiddenModelAuthorityIdentifier(identifier.Name) {
				t.Errorf("model-bound source %s contains forbidden authority %q", source.path, identifier.Name)
			}
			return true
		})
	}
}

func TestDesiredStateTransitionImportsRemainCodeOwnedAndClosed(t *testing.T) {
	t.Parallel()
	allowed := map[string]map[string]bool{
		"github.com/gryph/omnidex/internal/repository/changeapply": {
			"v3_artifact_candidate_authority.go": true,
			"v3_path_free_deletion_authority.go": true,
			"v3_path_free_deletion_compile.go":   true,
			"v3_repository_desired_prepare.go":   true,
			"v3_repository_desired_state.go":     true,
		},
		"github.com/gryph/omnidex/internal/omni": {},
	}
	seen := make(map[string]map[string]bool)
	for _, source := range desiredStateSources(t) {
		for _, imported := range source.imports {
			files, transitionPackage := allowed[imported]
			if !transitionPackage {
				continue
			}
			base := filepath.Base(source.path)
			if !files[base] {
				t.Errorf("desired-state source %s imports code-owned transition package %q outside its allowlist", base, imported)
				continue
			}
			if seen[imported] == nil {
				seen[imported] = make(map[string]bool)
			}
			seen[imported][base] = true
		}
	}
	for imported, files := range allowed {
		for file := range files {
			if !seen[imported][file] {
				t.Errorf("closed transition allowlist is stale: %s no longer imports %s", file, imported)
			}
		}
	}
}

func TestDesiredStateGoGenerationRetainsOneDeclarationParserBoundary(t *testing.T) {
	t.Parallel()
	source := parseArchitectureSource(t, "v3_go_fragment_generation.go")
	foundParser := false
	ast.Inspect(source.file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "ParseNewFunction" {
			foundParser = true
		}
		return true
	})
	if !foundParser {
		t.Fatal("desired-state declaration generation lost its bounded Go function parser")
	}
}

func TestDesiredStateArchitectureGuardFileRemainsFocused(t *testing.T) {
	for _, path := range []string{
		"v3_repository_desired_authority_architecture_test.go",
		"v3_repository_desired_authority_architecture_helpers_test.go",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if lines := strings.Count(string(raw), "\n") + 1; lines >= 300 {
			t.Fatalf("desired-state architecture guard %s has %d lines; split before 300", path, lines)
		}
	}
}
