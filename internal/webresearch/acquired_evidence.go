package webresearch

import (
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/modelcontext"
	"github.com/gryph/omnidex/internal/websearch"
)

// AcquiredEvidence retains the actual search and fetch observations. Local
// references select within this acquisition; they do not identify content.
// Projection settings are code-owned and are never model output.
type AcquiredEvidence struct {
	Discovery          websearch.CandidateReport
	Fetch              websearch.DocumentReport
	CandidateIDs       []websearch.CandidateID
	ProjectionBytes    int
	KnownArtifactPaths []string
}

func CaptureEvidence(result EvidenceResult, projectionBytes int, knownPaths []string) (AcquiredEvidence, error) {
	if len(result.Discovery) != 1 || len(result.Fetches) != 1 {
		return AcquiredEvidence{}, fmt.Errorf("web evidence requires the executed discovery and fetch reports")
	}
	acquired := AcquiredEvidence{
		Discovery:       cloneCandidateReport(result.Discovery[0]),
		Fetch:           cloneDocumentReport(result.Fetches[0]),
		ProjectionBytes: projectionBytes, KnownArtifactPaths: slices.Clone(knownPaths),
		CandidateIDs: make([]websearch.CandidateID, len(result.Evidence)),
	}
	for index, item := range result.Evidence {
		acquired.CandidateIDs[index] = item.CandidateID
	}
	selected, projected, err := acquired.Project()
	if err != nil {
		return AcquiredEvidence{}, err
	}
	if !slices.Equal(selected, result.Evidence) || !slices.Equal(projected, result.Projected) {
		return AcquiredEvidence{}, fmt.Errorf("selected web evidence differs from its actual fetched values and code projection")
	}
	return acquired, nil
}

// Project reconstructs only the retained selected evidence. It performs no
// acquisition or semantic selection and cannot reopen an accepted relation.
func (acquired AcquiredEvidence) Project() ([]Evidence, []ProjectedEvidence, error) {
	if acquired.ProjectionBytes < 256 || acquired.ProjectionBytes > 8192 {
		return nil, nil, fmt.Errorf("web evidence projection must fit 256..8192 bytes")
	}
	if err := validateAcquisitionQuery(acquired.Discovery.Query); err != nil {
		return nil, nil, err
	}
	if err := validateCandidateReportBounds(acquired.Discovery); err != nil {
		return nil, nil, err
	}
	if err := validateCandidateReport(acquired.Discovery, acquired.Discovery.Query, nil); err != nil {
		return nil, nil, err
	}
	if err := validateDocumentReportBounds(acquired.Fetch); err != nil {
		return nil, nil, err
	}
	if err := validateDocumentReport(acquired.Fetch, acquired.Discovery.Candidates, nil); err != nil {
		return nil, nil, err
	}
	selected, err := validateRelevanceDecision(RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: acquired.CandidateIDs,
	}, evidenceFromDocuments(acquired.Fetch.Documents), maxAcquisitionDocuments)
	if err != nil {
		return nil, nil, err
	}
	if err := validateEvidence(selected); err != nil {
		return nil, nil, err
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(acquired.KnownArtifactPaths)
	if err != nil {
		return nil, nil, err
	}
	projected, err := buildProjection(selected, acquired.ProjectionBytes, provenance)
	return selected, projected, err
}
