package assemblyline

import (
	"fmt"

	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

// RepositoryProjectionSources is the narrow resolver contract for the two
// repository semantic stations eligible for deterministic shadow projection.
type RepositoryProjectionSources struct {
	ResearchNeed string
	Evidence     *repositoryretrieval.EvidencePack
}

func ResolveRepositoryProjectionSources(job PortableJob) (RepositoryProjectionSources, error) {
	if err := job.Validate(); err != nil {
		return RepositoryProjectionSources{}, fmt.Errorf("resolve repository projection sources: %w", err)
	}
	switch job.Kind {
	case WorkRepositoryRetrieval:
		var input RepositoryRetrievalInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return RepositoryProjectionSources{}, err
		}
		return RepositoryProjectionSources{ResearchNeed: input.ResearchNeed}, nil
	case WorkRepositoryChangeSurface:
		var input RepositoryChangeSurfaceInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return RepositoryProjectionSources{}, err
		}
		return RepositoryProjectionSources{ResearchNeed: input.ResearchNeed, Evidence: &input.Evidence}, nil
	default:
		return RepositoryProjectionSources{}, fmt.Errorf(
			"portable work kind %q is not eligible for repository context projection", job.Kind,
		)
	}
}
