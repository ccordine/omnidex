package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func validateScrumJobAuthority(card ScrumCard, job model.JobDetails) error {
	if job.Job.ID <= 0 {
		return fmt.Errorf("Scrum output sync requires a positive typed job ID")
	}
	jobID := strconv.FormatInt(job.Job.ID, 10)
	if strings.TrimSpace(card.JobID) != jobID {
		return fmt.Errorf("Scrum output sync job %s differs from card job %q", jobID, card.JobID)
	}
	return nil
}
