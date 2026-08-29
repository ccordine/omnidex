package worker

import (
	"fmt"
	"strings"
)

func projectLaravelTaskStaticFiles(
	full directCodingProgram,
	stage directCodingProgram,
) ([]directCodingFileTask, error) {
	if full.StackID != laravelHTTPServiceAdapter || stage.StackID != laravelHTTPServiceAdapter {
		return nil, fmt.Errorf("Laravel task static projection requires the Laravel service stack")
	}
	if full.VersionProfileID != stage.VersionProfileID {
		return nil, fmt.Errorf("Laravel task static projection changed selected version profile")
	}
	profile, err := directCodingVersionProfileForProgram(full)
	if err != nil {
		return nil, err
	}
	paths, err := laravelContainerSourcePathsFor(stage.Source, []string{"src/Runtime.php"})
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
		return nil, fmt.Errorf("Laravel task stage requires one implementation and verification source")
	}
	for _, file := range stage.StaticFiles {
		if file.Path == laravelTestBootstrapPath || file.Path == laravelPlatformVerificationPath ||
			file.Path == phpServiceStateVerificationPath {
			paths = append(paths, file.Path)
		}
	}
	storage, err := deriveDirectCodingServiceStoragePlan(full.Workload, full.ServiceState)
	if err != nil {
		return nil, err
	}
	hasState := storage.RequiresPostgreSQL()
	hasHTML := phpServiceHasHTMLResponse(stage.ServiceEndpoints)
	projected := append([]directCodingFileTask(nil), stage.StaticFiles...)
	for index := range projected {
		switch projected[index].Path {
		case "Dockerfile":
			dockerfile, dockerErr := laravelDockerfile(profile, paths, hasHTML, hasState)
			if dockerErr != nil {
				return nil, dockerErr
			}
			projected[index].Content = dockerfile
		case ".dockerignore":
			projected[index].Content = laravelDockerignore(paths, hasHTML, hasState)
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
			return nil, fmt.Errorf("Laravel task static projection lacks %s", required)
		}
	}
	return projected, nil
}
