package worker

import (
	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/station"
)

func (session *directCodingSession) resolveNamedArtifactDeletionCandidates(
	featureQuotes []string,
	directives []assemblyline.ArtifactDirective,
	identities []assemblyline.ArtifactIdentity,
	analysis repositoryfacts.Analysis,
) ([]assemblyline.ArtifactDirective, error) {
	modelName, err := session.workerModel(station.CodingArtifactCandidateSelection)
	if err != nil {
		return nil, err
	}
	return resolveAmbiguousArtifactDeletion(
		directCodingWorkerRuntime(session), modelName, featureQuotes,
		directives, identities, session.repositoryIndex.Snapshot, analysis,
	)
}

func (session *directCodingSession) runNamedArtifactDeletion(
	authority string,
	partition assemblyline.RequirementPartitionDecision,
	directives []assemblyline.ArtifactDirective,
	identities []assemblyline.ArtifactIdentity,
	analysis repositoryfacts.Analysis,
) (string, error) {
	graph, err := compileExistingRepositoryDesiredGraph(
		authority, partition, nil, directives, identities,
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
