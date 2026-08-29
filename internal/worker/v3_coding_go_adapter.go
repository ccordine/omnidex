package worker

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"golang.org/x/mod/modfile"
)

const genericGoCommandLineAdapter = "go_command_line_capabilities_v1"

func compileGenericGoCommandLineBlueprint(
	packageName string,
	specification assemblyline.ApplicationSpecification,
	skills map[string]directCodingSkillBinding,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	target assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (assemblyline.SourceBlueprint, []directCodingFileTask, error) {
	if err := specification.Validate(); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if specification.Surface != assemblyline.ApplicationSurfaceCommandLine {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf(
			"generic Go command-line stack does not support surface %s", specification.Surface,
		)
	}
	profile, err := directCodingVersionProfileForTargetTree(target)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if err := validateDirectCodingSkillBindings(specification.Requirements, skills); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if err := validateDirectCodingCapabilityGraph(specification.Requirements, capabilities); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	contexts, err := directCodingApplicationTaskContexts(workload)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	documents, err := genericGoCommandLineDocuments(
		specification, skills, contexts, capabilities, coverage,
	)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	staticFiles, err := genericGoCommandLineStaticFiles(profile, packageName)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	return assemblyline.SourceBlueprint{Documents: documents}, staticFiles, nil
}

func genericGoCommandLineStaticFiles(
	profile directCodingProjectVersionProfile,
	packageName string,
) ([]directCodingFileTask, error) {
	version, err := directCodingVersionComponent(profile, "go")
	if err != nil {
		return nil, err
	}
	return []directCodingFileTask{
		{Path: ".gitignore", Content: packageName + "\n*.test\ncoverage.out\n"},
		{Path: "go.mod", Content: "module example.invalid/" + packageName + "\n\ngo " + version + "\n"},
	}, nil
}

func validateGoCommandLineAssembly(assembly directCodingAssembly) error {
	files := make(map[string]string, len(assembly.Files))
	mainFunctions := 0
	for _, file := range assembly.Files {
		files[file.Path] = file.Content
		if !strings.HasSuffix(file.Path, ".go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file.Path, file.Content, parser.AllErrors)
		if err != nil {
			return fmt.Errorf("parse assembled Go source %s: %w", file.Path, err)
		}
		if parsed.Name == nil || parsed.Name.Name != "main" {
			return fmt.Errorf("assembled Go source %s must use package main", file.Path)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil && function.Name.Name == "main" {
				mainFunctions++
			}
		}
	}
	manifest, exists := files["go.mod"]
	if !exists {
		return fmt.Errorf("Go command-line assembly lacks go.mod")
	}
	parsedModule, err := modfile.Parse("go.mod", []byte(manifest), nil)
	if err != nil {
		return fmt.Errorf("Go command-line assembly has an invalid module manifest: %w", err)
	}
	if parsedModule.Module == nil || parsedModule.Go == nil {
		return fmt.Errorf("Go command-line assembly requires module and go directives")
	}
	for _, required := range []string{"runtime.go", "main.go"} {
		if _, exists := files[required]; !exists {
			return fmt.Errorf("Go command-line assembly lacks code-owned artifact %s", required)
		}
	}
	if mainFunctions != 1 {
		return fmt.Errorf("Go command-line assembly requires exactly one main function, found %d", mainFunctions)
	}
	return nil
}

func goCommandLineVerificationCommands(
	_ directCodingProgram,
) ([]testCommand, error) {
	return goCommandLineVerificationCommandSet(), nil
}

func goCommandLineVerificationCommandSet() []testCommand {
	return []testCommand{
		{Family: "go", Name: "go", Args: []string{"test", "-count=1", "./..."}, Purpose: verificationTest},
		{Family: "go", Name: "go", Args: []string{"vet", "./..."}, Purpose: verificationSyntax},
		{Family: "go", Name: "go", Args: []string{"build", "./..."}, Purpose: verificationBuild},
	}
}
