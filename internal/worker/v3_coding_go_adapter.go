package worker

import (
	"github.com/gryph/omnidex/internal/assemblyline"
)

const genericGoCommandLineAdapter = "go_command_line_capabilities_v1"

func compileGenericGoCommandLineBlueprint(
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
	documents, err := genericGoCommandLineDocuments(
		specification, contexts, capabilities, coverage,
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
		{Path: "go.mod", Content: []byte("module example.invalid/" + packageName + "\n\ngo " + version + "\n"), Mode: 0o644},
	}, nil
}
