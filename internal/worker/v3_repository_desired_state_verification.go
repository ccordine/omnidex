package worker

import (
	"fmt"
	"sort"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

// desiredArtifactGraphGoVerificationCommands compiles repository verification
// from code-owned package authorities. Models receive neither commands nor
// their package arguments.
func desiredArtifactGraphGoVerificationCommands(
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	graph repositoryfacts.DesiredArtifactGraph,
) ([]testCommand, error) {
	if err := graph.Validate(snapshot, analysis); err != nil {
		return nil, fmt.Errorf("derive desired repository verification: %w", err)
	}
	if analysis.Adapter.Name != "go" {
		return nil, fmt.Errorf("desired repository verification does not support adapter %q", analysis.Adapter.Name)
	}
	packageSet := make(map[string]struct{}, len(graph.Artifacts))
	for _, artifact := range graph.Artifacts {
		placement, err := repositoryfacts.ResolveGoPackagePlacement(
			snapshot, analysis, artifact.PackageArtifactID,
		)
		if err != nil {
			return nil, fmt.Errorf("desired artifact %q verification package: %w", artifact.ID, err)
		}
		packageSet[repositoryGoPackageArgument(placement.Directory)] = struct{}{}
	}
	if len(packageSet) == 0 || len(packageSet) > maxExistingRepositoryVerificationPackages {
		return nil, fmt.Errorf(
			"desired repository verification requires 1-%d exact Go package scopes; received %d",
			maxExistingRepositoryVerificationPackages, len(packageSet),
		)
	}
	packages := make([]string, 0, len(packageSet))
	for packageArgument := range packageSet {
		packages = append(packages, packageArgument)
	}
	sort.Strings(packages)
	commands := make([]testCommand, 0, len(packages)+1)
	for _, packageArgument := range packages {
		commands = append(commands, testCommand{
			Family: "go", Name: "go",
			Args:    []string{"test", "-json", "-count=1", "-run", "^$", packageArgument},
			Purpose: verificationTest,
			RepositoryProof: &repositoryGoTestProof{
				Mode: repositoryGoProofPackage, Package: packageArgument,
			},
		})
	}
	commands = append(commands, testCommand{
		Family: "go", Name: "go", Args: []string{"test", "-json", "-count=1", "./..."},
		Purpose:         verificationTest,
		RepositoryProof: &repositoryGoTestProof{Mode: repositoryGoProofBroad, Package: "./..."},
	})
	for _, command := range commands {
		if err := validateRepositoryGoTestCommand(command); err != nil {
			return nil, fmt.Errorf(
				"desired repository verification command %q is invalid: %w",
				directCodingCommandLabel(command), err,
			)
		}
	}
	return commands, nil
}

func repositoryGoPackageArgument(directory string) string {
	if directory == "." {
		return "."
	}
	return "./" + directory
}
