package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func classifyRepositoryArtifactAbsence(
	runtime typedWorkerRuntime,
	modelName string,
	input assemblyline.RepositoryArtifactAbsenceInput,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.RepositoryArtifactAbsenceDecision, error) {
	job, err := assemblyline.NewRepositoryArtifactAbsenceJob(input)
	if err != nil {
		return assemblyline.RepositoryArtifactAbsenceDecision{}, err
	}
	return runDirectCodingSemanticLeafCall(
		runtime, modelName, "repository_artifact_absence", job, identities,
		func(raw string) (assemblyline.RepositoryArtifactAbsenceDecision, error) {
			return assemblyline.DecodeRepositoryArtifactAbsenceDecision(input, raw)
		},
		func(value assemblyline.RepositoryArtifactAbsenceDecision) error {
			if err := value.ValidateFor(input); err != nil {
				return fmt.Errorf("validate repository artifact absence: %w", err)
			}
			return nil
		},
	)
}

type repositoryArtifactAbsencePartition struct {
	MustBeAbsent []string
	NotExplicit  []string
}

func classifyRepositoryArtifactAbsenceQuotes(
	runtime typedWorkerRuntime,
	modelName string,
	quotes []string,
	identities []assemblyline.ArtifactIdentity,
) (repositoryArtifactAbsencePartition, error) {
	partition := repositoryArtifactAbsencePartition{
		MustBeAbsent: make([]string, 0, len(quotes)),
		NotExplicit:  make([]string, 0, len(quotes)),
	}
	for _, quote := range quotes {
		decision, err := classifyRepositoryArtifactAbsence(
			runtime, modelName,
			assemblyline.RepositoryArtifactAbsenceInput{RequirementQuote: quote}, identities,
		)
		if err != nil {
			return repositoryArtifactAbsencePartition{}, err
		}
		switch decision.Relation {
		case assemblyline.RepositoryArtifactMustBeAbsent:
			partition.MustBeAbsent = append(partition.MustBeAbsent, quote)
		case assemblyline.RepositoryArtifactAbsenceNotExplicit:
			partition.NotExplicit = append(partition.NotExplicit, quote)
		default:
			return repositoryArtifactAbsencePartition{}, fmt.Errorf(
				"repository artifact absence relation %q escaped validated authority",
				decision.Relation,
			)
		}
	}
	return partition, nil
}
