package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/station"
)

func (session *directCodingSession) classifyPathFreeArtifactAbsence(
	featureQuotes []string,
	directives []assemblyline.ArtifactDirective,
	identities []assemblyline.ArtifactIdentity,
) (repositoryArtifactAbsencePartition, error) {
	pathFreeQuotes, err := pathFreeArtifactAbsenceQuotes(featureQuotes, directives)
	if err != nil {
		return repositoryArtifactAbsencePartition{}, err
	}
	if len(pathFreeQuotes) == 0 {
		return repositoryArtifactAbsencePartition{}, nil
	}
	modelName, err := session.workerModel(station.CodingRepositoryArtifactAbsence)
	if err != nil {
		return repositoryArtifactAbsencePartition{}, err
	}
	partition, err := classifyRepositoryArtifactAbsenceQuotes(
		directCodingWorkerRuntime(session), modelName, pathFreeQuotes, identities,
	)
	if err != nil {
		return repositoryArtifactAbsencePartition{}, err
	}
	return partition, nil
}

func pathFreeArtifactAbsenceQuotes(
	featureQuotes []string,
	directives []assemblyline.ArtifactDirective,
) ([]string, error) {
	named := make(map[string]struct{}, len(directives))
	for _, directive := range directives {
		if directive.Disposition != assemblyline.ArtifactForbid &&
			directive.Disposition != assemblyline.ArtifactAbsenceCandidate {
			continue
		}
		quote, err := exactArtifactAbsenceRequirementQuote(directive.Token, featureQuotes)
		if err != nil {
			return nil, err
		}
		named[quote] = struct{}{}
	}
	pathFree := make([]string, 0, len(featureQuotes)-len(named))
	for _, quote := range featureQuotes {
		if _, hasNamedArtifact := named[quote]; !hasNamedArtifact {
			pathFree = append(pathFree, quote)
		}
	}
	return pathFree, nil
}

func (session *directCodingSession) runPathFreeArtifactDeletion(
	authority string,
	featureQuotes []string,
	absenceQuotes []string,
	identities []assemblyline.ArtifactIdentity,
	analysis repositoryfacts.Analysis,
) (string, error) {
	if len(absenceQuotes) != len(featureQuotes) {
		return "", fmt.Errorf(
			"mixed path-free deletion and create or modify requirements are unsupported",
		)
	}
	if len(absenceQuotes) != 1 {
		return "", fmt.Errorf(
			"multiple path-free artifact absence requirements are unsupported",
		)
	}
	modelName, err := session.workerModel(station.CodingArtifactCandidateSelection)
	if err != nil {
		return "", err
	}
	selected, err := selectPathFreeDeletionAuthority(
		directCodingWorkerRuntime(session), modelName, absenceQuotes[0], identities,
		session.repositoryIndex.Snapshot, analysis,
	)
	if err != nil {
		return "", err
	}
	graph, err := compilePathFreeDesiredArtifactDeletion(
		session.runtime.ctx, authority, absenceQuotes[0], selected,
		session.repositoryIndex.Snapshot, analysis,
	)
	if err != nil {
		return "", err
	}
	if err := session.recordDesiredRepositoryGraph(graph); err != nil {
		return "", err
	}
	return session.runExistingRepositoryDesiredState(graph, analysis)
}
