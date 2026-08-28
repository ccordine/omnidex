package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/station"
)

func (session *directCodingSession) classifyPathFreeArtifactTruth(
	featureQuotes []string,
	directives []assemblyline.ArtifactDirective,
	identities []assemblyline.ArtifactIdentity,
) (knownArtifactTruthPartition, error) {
	modelName, err := session.workerModel(station.CodingKnownArtifactTruth)
	if err != nil {
		return knownArtifactTruthPartition{}, err
	}
	partition, err := classifyKnownArtifactTruthQuotes(
		directCodingWorkerRuntime(session), modelName, featureQuotes, identities,
	)
	if err != nil {
		return knownArtifactTruthPartition{}, err
	}
	if err := validateDirectArtifactAbsenceTruth(
		featureQuotes, directives, partition.MustBeAbsent,
	); err != nil {
		return knownArtifactTruthPartition{}, err
	}
	return partition, nil
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
