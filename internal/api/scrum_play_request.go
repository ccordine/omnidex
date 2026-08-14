package api

import (
	"fmt"
	"net/http"
)

type scrumPlayRequest struct {
	Pivot             bool                      `json:"pivot"`
	ExpectedUpdatedAt requiredScrumCardRevision `json:"expected_updated_at"`
}

type scrumPauseRequest struct {
	ExpectedUpdatedAt requiredScrumCardRevision `json:"expected_updated_at"`
}

func decodeScrumPlayRequest(w http.ResponseWriter, r *http.Request) (scrumPlayRequest, error) {
	var request scrumPlayRequest
	if err := decodeExactScrumCardStateAction(w, r, &request, "Scrum card play"); err != nil {
		return scrumPlayRequest{}, err
	}
	if !request.ExpectedUpdatedAt.Present {
		return scrumPlayRequest{}, fmt.Errorf("Scrum card play expected_updated_at is required")
	}
	return request, nil
}

func decodeScrumPauseRequest(w http.ResponseWriter, r *http.Request) (scrumPauseRequest, error) {
	var request scrumPauseRequest
	if err := decodeExactScrumCardStateAction(w, r, &request, "Scrum card pause"); err != nil {
		return scrumPauseRequest{}, err
	}
	if !request.ExpectedUpdatedAt.Present {
		return scrumPauseRequest{}, fmt.Errorf("Scrum card pause expected_updated_at is required")
	}
	return request, nil
}
