package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

const chatJobStateComponentPrefix = "/v1/ui/chat/jobs/"

type chatJobStateAuthority struct {
	ID                int64      `json:"id"`
	Status            string     `json:"status"`
	Error             string     `json:"error,omitempty"`
	CurrentGeneration int64      `json:"current_generation"`
	UpdatedAt         time.Time  `json:"updated_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
}

type chatJobStepAuthority struct {
	ID         int64  `json:"id"`
	Action     string `json:"action"`
	Status     string `json:"status"`
	Generation int64  `json:"generation"`
}

type chatJobStateResponse struct {
	Job   chatJobStateAuthority  `json:"job"`
	Steps []chatJobStepAuthority `json:"steps"`
	HTML  chatComponentHTML      `json:"html"`
}

func newChatJobStateResponse(presentation queue.JobPresentation) (chatJobStateResponse, error) {
	bundle, err := renderChatJobStateBundle(presentation)
	if err != nil {
		return chatJobStateResponse{}, err
	}
	steps := make([]chatJobStepAuthority, len(presentation.Steps))
	for index, step := range presentation.Steps {
		steps[index] = chatJobStepAuthority{
			ID: step.ID, Action: step.Action, Status: step.Status, Generation: step.Generation,
		}
	}
	return chatJobStateResponse{
		Job: chatJobStateAuthority{
			ID: presentation.Job.ID, Status: presentation.Job.Status, Error: presentation.Job.Error,
			CurrentGeneration: presentation.Job.CurrentGeneration,
			UpdatedAt:         presentation.Job.UpdatedAt, CompletedAt: presentation.Job.CompletedAt,
		},
		Steps: steps,
		HTML: chatComponentHTML{Bundle: bundle},
	}, nil
}

func parseChatJobStateComponentID(path string) (int64, error) {
	if !strings.HasPrefix(path, chatJobStateComponentPrefix) {
		return 0, fmt.Errorf("chat job state path is outside the registered component prefix")
	}
	idText := strings.TrimPrefix(path, chatJobStateComponentPrefix)
	if idText == "" || strings.Contains(idText, "/") || idText[0] < '1' || idText[0] > '9' {
		return 0, fmt.Errorf("chat job state path requires one canonical positive job ID")
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != idText {
		return 0, fmt.Errorf("chat job state path requires one canonical positive job ID")
	}
	return id, nil
}

func (s *Server) handleChatJobStateComponent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.RawQuery != "" {
		writeError(w, http.StatusBadRequest, "chat job state does not accept query fields")
		return
	}
	jobID, err := parseChatJobStateComponentID(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	presentation, err := s.repo.CurrentJobPresentation(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	payload, err := newChatJobStateResponse(presentation)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeChatComponentJSON(w, payload)
}
