package api

import (
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

func validateSameJobAuthority(expectedID int64, job model.Job) error {
	if expectedID <= 0 {
		return fmt.Errorf("feedback authority requires a positive expected job id")
	}
	if job.ID != expectedID {
		return fmt.Errorf("feedback authority expected job %d, repository returned job %d", expectedID, job.ID)
	}
	return nil
}
