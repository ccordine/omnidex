package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/queue"
)

type scrumCardItemRequest struct {
	Action            queue.ScrumCardItemAction `json:"action"`
	ExpectedUpdatedAt time.Time                 `json:"expected_updated_at"`
	ItemID            string                    `json:"item_id,omitempty"`
	Text              string                    `json:"text,omitempty"`
	Done              *bool                     `json:"done,omitempty"`
}

func (s *Server) handleScrumCardItem(w http.ResponseWriter, r *http.Request, cardID string, collection queue.ScrumCardItemCollection) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Scrum card item mutation only accepts POST")
		return
	}
	projectID, err := decodeScrumMutationProjectID(r, "Scrum card item mutation")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	request, err := decodeScrumCardItemRequest(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.repo.MutateScrumCardItem(r.Context(), queue.ScrumCardItemMutation{
		ProjectID: projectID, CardID: cardID, ExpectedUpdatedAt: request.ExpectedUpdatedAt,
		Collection: collection, Action: request.Action, ItemID: request.ItemID, Text: request.Text, Done: request.Done,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, queue.ErrScrumCardVersionConflict) {
			status = http.StatusConflict
		} else if errors.Is(err, queue.ErrScrumCardNotFound) || errors.Is(err, queue.ErrScrumCardItemNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	card, err := dbScrumCardToAPI(updated)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	responseCard, err := scrumCardActionProjection(card)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": responseCard})
}

func decodeScrumCardItemRequest(w http.ResponseWriter, r *http.Request) (scrumCardItemRequest, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return scrumCardItemRequest{}, fmt.Errorf("read Scrum card item mutation: %w", err)
	}
	if !utf8.Valid(raw) {
		return scrumCardItemRequest{}, fmt.Errorf("Scrum card item mutation must be valid UTF-8")
	}
	if err := exactjson.ValidateObject(raw, scrumCardItemRequest{}, "Scrum card item mutation"); err != nil {
		return scrumCardItemRequest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request scrumCardItemRequest
	if err := decoder.Decode(&request); err != nil {
		return scrumCardItemRequest{}, fmt.Errorf("decode Scrum card item mutation: %w", err)
	}
	if err := requireJSONEOF(decoder, "Scrum card item mutation"); err != nil {
		return scrumCardItemRequest{}, err
	}
	if request.Action == queue.ScrumCardItemAdd {
		request.Text = strings.TrimSpace(request.Text)
	}
	probe := queue.ScrumCardItemMutation{
		ProjectID: 1, CardID: "probe", ExpectedUpdatedAt: request.ExpectedUpdatedAt,
		Collection: queue.ScrumCardChecklist, Action: request.Action,
		ItemID: request.ItemID, Text: request.Text, Done: request.Done,
	}
	if err := probe.ValidateForTransport(); err != nil {
		return scrumCardItemRequest{}, err
	}
	return request, nil
}
