package assemblyline

import (
	"fmt"
	"strings"
)

type PortableResult struct {
	Candidate string `json:"candidate"`
}

func (result PortableResult) ValidateFor(job PortableJob) error {
	if err := job.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(result.Candidate) == "" {
		return fmt.Errorf("portable result candidate is empty")
	}
	maximum, err := PortableResponseMaximumBytesForJob(job)
	if err != nil {
		return err
	}
	if maximum > 0 && len(result.Candidate) > maxPortableRawCandidateBytes {
		return fmt.Errorf(
			"portable result candidate exceeds gross resource ceiling of %d bytes",
			maxPortableRawCandidateBytes,
		)
	}
	if maximum > 0 && len(result.Candidate) > maximum {
		return fmt.Errorf(
			"portable result candidate exceeds %s response ceiling of %d bytes",
			job.Kind, maximum,
		)
	}
	return nil
}
