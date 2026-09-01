package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type directCodingApplicationInputs struct {
	RequestAuthority    directCodingApplicationRequestAuthority
	ApplicationContext  assemblyline.ApplicationContext
	Identities          []assemblyline.ArtifactIdentity
	Runtime             typedWorkerRuntime
	RequirementModel    string
	ResultRelationModel string
}

func (s *directCodingSession) prepareApplicationInputs() (directCodingApplicationInputs, error) {
	var zero directCodingApplicationInputs
	authority := s.directCodingAuthority()
	provenance, err := objectiveInstructionPathProvenance(
		s.runtime.ctx, s.root, authority,
	)
	if err != nil {
		return zero, fmt.Errorf("derive current-tree artifact provenance: %w", err)
	}
	s.pathProvenance = provenance
	redacted, identities, err := assemblyline.RedactArtifactIdentities(authority, provenance)
	if err != nil {
		return zero, err
	}
	requestAuthority, err := newDirectCodingApplicationRequestAuthority(authority, redacted)
	if err != nil {
		return zero, err
	}
	requirementModel, err := s.workerModel(station.CodingRequirements)
	if err != nil {
		return zero, err
	}
	resultRelationModel, err := s.workerModel(station.CodingRequirementResultRelation)
	if err != nil {
		return zero, err
	}
	applicationContext, err := assemblyline.BootstrapApplicationContext(redacted)
	if err != nil {
		return zero, err
	}
	return directCodingApplicationInputs{
		RequestAuthority:    requestAuthority,
		ApplicationContext:  applicationContext,
		Identities:          identities,
		Runtime:             directCodingWorkerRuntime(s),
		RequirementModel:    requirementModel,
		ResultRelationModel: resultRelationModel,
	}, nil
}
