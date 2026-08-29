package api

import (
	"fmt"
	"net/http"
)

type scrumCardDeleteRequest struct {
	ExpectedUpdatedAt requiredScrumCardRevision `json:"expected_updated_at"`
}

func decodeScrumCardDeleteRequest(w http.ResponseWriter, r *http.Request) (scrumCardDeleteRequest, error) {
	var request scrumCardDeleteRequest
	if err := decodeExactScrumCardStateAction(w, r, &request, "Scrum card delete"); err != nil {
		return scrumCardDeleteRequest{}, err
	}
	if !request.ExpectedUpdatedAt.Present {
		return scrumCardDeleteRequest{}, fmt.Errorf("Scrum card delete expected_updated_at is required")
	}
	return request, nil
}
