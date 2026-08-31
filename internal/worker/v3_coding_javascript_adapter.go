package worker

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const genericJavaScriptCommandLineAdapter = "javascript_command_line_capabilities_v1"

func compileGenericJavaScriptCommandLineBlueprint(
	packageName string,
	specification assemblyline.ApplicationSpecification,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	profile directCodingProjectVersionProfile,
	target assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (assemblyline.SourceBlueprint, []directCodingFileTask, error) {
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
		{Path: "package.json", Content: append(encoded, '\n'), Mode: 0o644},
	}, nil
}
