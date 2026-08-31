package worker

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const genericJavaScriptCommandLineAdapter = "javascript_command_line_capabilities_v1"

func compileGenericJavaScriptCommandLineBlueprint(
	packageName string,
	specification assemblyline.ApplicationSpecification,
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
			"generic JavaScript command-line stack does not support surface %s", specification.Surface,
		)
	}
	profile, err := directCodingVersionProfileForTargetTree(target)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if err := validateDirectCodingCapabilityGraph(specification.Requirements, capabilities); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	contexts, err := directCodingApplicationTaskContexts(workload)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	documents, err := genericJavaScriptCommandLineDocuments(
		profile, specification, contexts, capabilities, coverage,
	)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	staticFiles, err := genericJavaScriptCommandLineStaticFiles(profile, packageName)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	return assemblyline.SourceBlueprint{Documents: documents}, staticFiles, nil
}

func genericJavaScriptCommandLineStaticFiles(
	profile directCodingProjectVersionProfile,
	packageName string,
) ([]directCodingFileTask, error) {
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return nil, err
	}
	manifest := map[string]any{
		"name": packageName, "private": true, "type": "module",
		"engines": map[string]string{"node": node},
		"scripts": map[string]string{
			"build": "node --check main.mjs",
		},
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode code-owned JavaScript manifest: %w", err)
	}
	return []directCodingFileTask{
		{Path: ".gitignore", Content: "node_modules\n"},
		{Path: "package.json", Content: string(encoded) + "\n"},
	}, nil
}

func validateJavaScriptCommandLineAssembly(assembly directCodingAssembly) error {
	files := make(map[string]string, len(assembly.Files))
	for _, file := range assembly.Files {
		files[file.Path] = file.Content
	}
	for _, required := range []string{"package.json", "runtime.mjs", "main.mjs"} {
		if _, exists := files[required]; !exists {
			return fmt.Errorf("JavaScript command-line assembly lacks code-owned artifact %s", required)
		}
	}
	var manifest struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(files["package.json"]), &manifest); err != nil {
		return fmt.Errorf("decode JavaScript command-line manifest: %w", err)
	}
	if manifest.Type != "module" {
		return fmt.Errorf("JavaScript command-line manifest must select ECMAScript modules")
	}
	for artifactPath := range files {
		if strings.HasSuffix(artifactPath, ".mjs") && path.Dir(artifactPath) != "." {
			return fmt.Errorf("JavaScript command-line source %s must belong to the root module", artifactPath)
		}
	}
	return nil
}
