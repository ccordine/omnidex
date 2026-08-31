package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

const defaultJobHistoryPageSize = 50

func parseJobHistoryRequest(request *http.Request) (queue.JobHistoryRequest, error) {
	values := request.URL.Query()
	for key := range values {
		switch key {
		case "stream", "limit", "cursor":
		default:
			return queue.JobHistoryRequest{}, fmt.Errorf("unknown job history query field %q", key)
		}
	}
	if len(values["stream"]) != 1 {
		return queue.JobHistoryRequest{}, fmt.Errorf("job history requires exactly one stream")
	}
	stream := queue.JobHistoryStream(values["stream"][0])
	switch stream {
	case queue.JobHistoryGenerations, queue.JobHistorySteps, queue.JobHistoryEvidence:
	default:
		return queue.JobHistoryRequest{}, fmt.Errorf("job history stream %q is not registered", stream)
	}

	limit := defaultJobHistoryPageSize
	if provided, exists := values["limit"]; exists {
		if len(provided) != 1 || provided[0] == "" {
			return queue.JobHistoryRequest{}, fmt.Errorf("job history limit must be one exact integer")
		}
		parsed, err := strconv.Atoi(provided[0])
		if err != nil || parsed < 1 || parsed > queue.MaxJobHistoryPageSize {
			return queue.JobHistoryRequest{}, fmt.Errorf(
				"job history limit must be between 1 and %d", queue.MaxJobHistoryPageSize,
			)
		}
		limit = parsed
	}

	cursor := ""
	if provided, exists := values["cursor"]; exists {
		if len(provided) != 1 || provided[0] == "" {
			return queue.JobHistoryRequest{}, fmt.Errorf("job history cursor must be one nonempty opaque value")
		}
		cursor = provided[0]
	}
	return queue.JobHistoryRequest{Stream: stream, Limit: limit, Cursor: cursor}, nil
}

func (s *Server) jobHistory(w http.ResponseWriter, request *http.Request, jobID int64) {
	w.Header().Set("Cache-Control", "no-store")
	historyRequest, err := parseJobHistoryRequest(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := s.repo.ReadJobHistoryPage(request.Context(), jobID, historyRequest)
	if err != nil {
		switch {
		case errors.Is(err, queue.ErrInvalidJobHistoryRequest):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, pgx.ErrNoRows):
			writeError(w, http.StatusNotFound, "job not found")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, page)
}
