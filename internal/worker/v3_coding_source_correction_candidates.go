package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func selectDirectCodingSourceCorrection(job directCodingLanguageGenerationJob, provenance assemblyline.ArtifactIdentityProvenance, correction assemblyline.SourceBodyCorrection, response string) (string, error) {
	bodies, err := correction.ApplyCandidates(response)
	if err != nil {
		return "", err
	}
	for _, body := range bodies {
		candidate, err := job.Validate(job.Input, body)
		if err != nil {
			continue
		}
		pathSource := body
		if job.Input.Language == "go" {
			pathSource = candidate
		}
		if err := validateDirectCodingLanguageFragmentCandidatePathBoundary(job.Input.Language, provenance, pathSource); err == nil {
			return body, nil
		}
	}
	// Retain the first bounded candidate for the ordinary exact-defect loop.
	return bodies[0], nil
}
