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
	_, err := PortableResponseMaximumBytesForJob(job)
	if err != nil {
		return err
	}

	return nil
}
