package worker

import (
	"fmt"
	"path"
	"strings"

	"github.com/gryph/omnidex/internal/gofragment"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func desiredRepositoryGoVersionProfile(
	graph repositoryfacts.DesiredArtifactGraph,
	snapshot repositoryfacts.Snapshot,
	analyses []repositoryfacts.Analysis,
	artifact repositoryfacts.DesiredGoArtifact,
) (directCodingProjectVersionProfile, error) {
	compiled, err := gofragment.CompileNewFunctionSignature(artifact.Signature)
	if err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	fileName, err := deterministicGoSourceName(compiled.Name)
	if err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	analysis, err := exactRepositoryChangeAnalysis(
		analyses, desiredGraphAnalysisID(graph, analyses),
	)
	if err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	placement, err := repositoryfacts.ResolveGoPackagePlacement(
		snapshot, analysis, artifact.PackageArtifactID,
	)
	if err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	targetPath := fileName
	if placement.Directory != "." {
		targetPath = path.Join(placement.Directory, fileName)
	}
	return repositoryGoVersionProfile(snapshot, targetPath)
}

func repositoryGoVersionProfile(
	snapshot repositoryfacts.Snapshot,
	targetPath string,
) (directCodingProjectVersionProfile, error) {
	manifestPath, err := nearestRepositoryGoManifest(snapshot, targetPath)
	if err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	source, err := directCodingTargetTreeExistingSource(snapshot.Root, manifestPath)
	if err != nil {
		return directCodingProjectVersionProfile{}, fmt.Errorf("read existing repository version authority %s: %w", manifestPath, err)
	}
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	profiles, err := directCodingVersionProfilesForStack(stack)
	if err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	compatible, applicable, err := matchDirectCodingVersionProfiles(
		profiles, map[string]string{"go.mod": source},
	)
	if err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	if len(compatible) != 1 {
		return directCodingProjectVersionProfile{}, fmt.Errorf(
			"existing Go manifest %s matches %d compatible profiles from %d applicable profiles",
			manifestPath, len(compatible), applicable,
		)
	}
	return compatible[0], nil
}

func repositorySnapshotFilePath(
	snapshot repositoryfacts.Snapshot,
	fileID string,
) (string, error) {
	match := ""
	for _, file := range snapshot.Files {
		if file.ID != fileID {
			continue
		}
		if match != "" {
			return "", fmt.Errorf("repository snapshot repeats file ID %q", fileID)
		}
		if file.Kind != repositoryfacts.EntryRegular {
			return "", fmt.Errorf("repository file ID %q is not a regular file", fileID)
		}
		match = file.Path
	}
	if match == "" {
		return "", fmt.Errorf("repository snapshot lacks file ID %q", fileID)
	}
	return match, nil
}

func nearestRepositoryGoManifest(
	snapshot repositoryfacts.Snapshot,
	targetPath string,
) (string, error) {
	targetDir := path.Dir(targetPath)
	best := ""
	for _, file := range snapshot.Files {
		if file.Kind != repositoryfacts.EntryRegular || path.Base(file.Path) != "go.mod" {
			continue
		}
		directory := path.Dir(file.Path)
		if directory != "." && targetDir != directory &&
			!strings.HasPrefix(targetDir, directory+"/") {
			continue
		}
		if best == "" || len(directory) > len(path.Dir(best)) {
			best = file.Path
		}
	}
	if best == "" {
		return "", fmt.Errorf("desired Go target %s has no containing go.mod authority", targetPath)
	}
	return best, nil
}
