package worker

import (
	"fmt"
	"strings"
)

func projectPHPServiceTaskStaticFiles(
	full directCodingProgram,
	stage directCodingProgram,
) ([]directCodingFileTask, error) {
	if full.StackID != genericPHPServiceAdapter || stage.StackID != genericPHPServiceAdapter {
		return nil, fmt.Errorf("PHP task static projection requires the PHP service stack")
	}
	if full.VersionProfileID != stage.VersionProfileID {
		return nil, fmt.Errorf("PHP task static projection changed selected version profile")
	}
	profile, err := directCodingVersionProfileForProgram(full)
	if err != nil {
		return nil, err
	}
	storage, err := deriveDirectCodingServiceStoragePlan(full.Workload, full.ServiceState)
	if err != nil {
		return nil, err
	}
	hasState := storage.RequiresPostgreSQL()
	paths, err := phpServiceContainerSourcePathsFor(stage.Source, []string{"src/Runtime.php"})
	if err != nil {
		return nil, err
	}
	implementations, verifications := 0, 0
	for _, artifactPath := range paths {
		if phpServiceImplementationPath.MatchString(artifactPath) {
			implementations++
		}
		if phpServiceVerificationPath.MatchString(artifactPath) {
			verifications++
		}
	}
	if implementations != 1 || verifications != 1 {
		return nil, fmt.Errorf(
			"PHP task static projection requires one implementation and verification source",
		)
	}
	hasHTML := phpServiceHasHTMLResponse(stage.ServiceEndpoints)
	projected := append([]directCodingFileTask(nil), stage.StaticFiles...)
	for index := range projected {
		switch projected[index].Path {
		case "Dockerfile":
			dockerfile, dockerErr := phpServiceDockerfile(profile, paths, hasHTML, hasState)
			if dockerErr != nil {
				return nil, dockerErr
			}
			projected[index].Content = dockerfile
		case ".dockerignore":
			projected[index].Content = phpServiceDockerignore(paths, hasHTML, hasState)
		}
	}
	for _, required := range []string{"Dockerfile", ".dockerignore"} {
		found := false
		for _, file := range projected {
			if file.Path == required && strings.TrimSpace(file.Content) != "" {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("PHP task static projection lacks %s", required)
		}
	}
	return projected, nil
}
