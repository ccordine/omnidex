package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	maxScrumCardTicketActionBodyBytes int64 = 16 * 1024
	maxAssembledScrumCardTicketBytes        = 64 * 1024
)

type scrumCardTicketAction string

const (
	scrumCardTicketGenerate  scrumCardTicketAction = "generate"
	scrumCardTicketElaborate scrumCardTicketAction = "elaborate"
)

type requiredScrumTicketString struct {
	Present bool
	Value   string
}

func (field *requiredScrumTicketString) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("Scrum card ticket action fields must not be null")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("Scrum card ticket action fields must be strings: %w", err)
	}
	field.Present = true
	field.Value = value
	return nil
}

type scrumCardTicketActionBody struct {
	Action            requiredScrumTicketString `json:"action"`
	ExpectedUpdatedAt requiredScrumTicketString `json:"expected_updated_at"`
	Elaboration       requiredScrumTicketString `json:"elaboration"`
}

type scrumCardTicketActionRequest struct {
	Action            scrumCardTicketAction
	ExpectedUpdatedAt time.Time
	Elaboration       string
}

func decodeScrumCardTicketActionRequest(
	w http.ResponseWriter,
	r *http.Request,
) (scrumCardTicketActionRequest, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxScrumCardTicketActionBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return scrumCardTicketActionRequest{}, fmt.Errorf(
				"Scrum card ticket action exceeds the %d-byte transport bound",
				maxScrumCardTicketActionBodyBytes,
			)
		}
		return scrumCardTicketActionRequest{}, fmt.Errorf("read Scrum card ticket action: %w", err)
	}
	if !utf8.Valid(raw) {
		return scrumCardTicketActionRequest{}, fmt.Errorf("Scrum card ticket action must be valid UTF-8")
	}
	if err := exactjson.ValidateObject(raw, scrumCardTicketActionBody{}, "Scrum card ticket action"); err != nil {
		return scrumCardTicketActionRequest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var body scrumCardTicketActionBody
	if err := decoder.Decode(&body); err != nil {
		return scrumCardTicketActionRequest{}, fmt.Errorf("decode Scrum card ticket action: %w", err)
	}
	if err := requireJSONEOF(decoder, "Scrum card ticket action"); err != nil {
		return scrumCardTicketActionRequest{}, err
	}
	return body.validate()
}

func (body scrumCardTicketActionBody) validate() (scrumCardTicketActionRequest, error) {
	if !body.Action.Present || strings.TrimSpace(body.Action.Value) == "" {
		return scrumCardTicketActionRequest{}, fmt.Errorf("Scrum card ticket action is required")
	}
	if !body.ExpectedUpdatedAt.Present || strings.TrimSpace(body.ExpectedUpdatedAt.Value) == "" {
		return scrumCardTicketActionRequest{}, fmt.Errorf("Scrum card ticket expected_updated_at is required")
	}
	for name, value := range map[string]string{
		"action": body.Action.Value, "expected_updated_at": body.ExpectedUpdatedAt.Value,
		"elaboration": body.Elaboration.Value,
	} {
		if strings.ContainsRune(value, '\x00') {
			return scrumCardTicketActionRequest{}, fmt.Errorf("Scrum card ticket %s contains a forbidden NUL character", name)
		}
	}
	action := scrumCardTicketAction(strings.TrimSpace(body.Action.Value))
	if action != scrumCardTicketGenerate && action != scrumCardTicketElaborate {
		return scrumCardTicketActionRequest{}, fmt.Errorf("Scrum card ticket action %q is not registered", action)
	}
	revision, err := time.Parse(time.RFC3339Nano, body.ExpectedUpdatedAt.Value)
	if err != nil {
		return scrumCardTicketActionRequest{}, fmt.Errorf("Scrum card ticket expected_updated_at is invalid: %w", err)
	}
	if body.ExpectedUpdatedAt.Value != revision.UTC().Format(time.RFC3339Nano) {
		return scrumCardTicketActionRequest{}, fmt.Errorf("Scrum card ticket expected_updated_at must be canonical UTC")
	}
	elaboration := strings.TrimSpace(body.Elaboration.Value)
	if action == scrumCardTicketGenerate && body.Elaboration.Present {
		return scrumCardTicketActionRequest{}, fmt.Errorf("Scrum card ticket generate must not include elaboration")
	}
	if action == scrumCardTicketElaborate && elaboration == "" {
		return scrumCardTicketActionRequest{}, fmt.Errorf("Scrum card ticket elaborate requires elaboration")
	}
	return scrumCardTicketActionRequest{Action: action, ExpectedUpdatedAt: revision, Elaboration: elaboration}, nil
}

func (s *Server) handleScrumCardTicket(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	request, err := decodeScrumCardTicketActionRequest(w, r)
	if err != nil {
		writeError(w, scrumCardTicketBodyStatus(err), err.Error())
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "postgres repository is required for Scrum")
		return
	}
	projectID, err := s.resolveProjectID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	current, err := s.repo.GetScrumCard(r.Context(), projectID, cardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	card, err := dbScrumCardToAPI(current)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("decode Scrum card ticket source: %v", err))
		return
	}
	storedElaboration := card.CardPrompt
	card.CardPrompt = ""
	if request.Action == scrumCardTicketElaborate {
		storedElaboration = request.Elaboration
		card.CardPrompt = request.Elaboration
	}
	ticket, err := assembleScrumCardTicket(card)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	updated, err := s.repo.UpdateScrumCardTicket(r.Context(), queue.ScrumCardTicketMutation{
		ProjectID: projectID, CardID: cardID, ExpectedUpdatedAt: request.ExpectedUpdatedAt,
		Ticket: ticket, Elaboration: storedElaboration,
	})
	if err != nil {
		writeScrumCardTicketMutationError(w, err)
		return
	}
	result, err := dbScrumCardToAPI(updated)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("decode updated Scrum card ticket: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": result})
}

func scrumCardTicketBodyStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) || strings.Contains(err.Error(), "transport bound") {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func writeScrumCardTicketMutationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, queue.ErrScrumCardVersionConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, queue.ErrScrumCardNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func assembleScrumCardTicket(card ScrumCard) (string, error) {
	var ticket strings.Builder
	writeTicketSection(&ticket, "# "+singleLine(card.Title))
	writeTicketSection(&ticket, "## Objective", strings.TrimSpace(card.Description))
	writeTicketItems(&ticket, "## Checklist", card.Checklist)
	writeTicketItems(&ticket, "## Test criteria", card.TestCriteria)
	writeTicketValues(&ticket, "## Reference files", card.RefFiles, true)
	writeTicketValues(&ticket, "## Tags", card.Tags, false)
	writeTicketSection(&ticket, "## Elaboration", strings.TrimSpace(card.CardPrompt))
	result := strings.TrimSpace(ticket.String()) + "\n"
	if len(result) > maxAssembledScrumCardTicketBytes {
		return "", fmt.Errorf("assembled Scrum card ticket exceeds the %d-byte bound", maxAssembledScrumCardTicketBytes)
	}
	return result, nil
}

func writeTicketSection(ticket *strings.Builder, values ...string) {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			if ticket.Len() > 0 {
				ticket.WriteString("\n\n")
			}
			ticket.WriteString(value)
		}
	}
}

func writeTicketItems(ticket *strings.Builder, heading string, items []ScrumChecklistItem) {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		if text := singleLine(item.Text); text != "" {
			mark := " "
			if item.Done {
				mark = "x"
			}
			lines = append(lines, "- ["+mark+"] "+text)
		}
	}
	if len(lines) > 0 {
		writeTicketSection(ticket, heading, strings.Join(lines, "\n"))
	}
}

func writeTicketValues(ticket *strings.Builder, heading string, values []string, code bool) {
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if value = singleLine(value); value != "" {
			if code {
				value = "`" + strings.ReplaceAll(value, "`", "\\`") + "`"
			}
			clean = append(clean, value)
		}
	}
	sort.Strings(clean)
	if len(clean) > 0 {
		for index := range clean {
			clean[index] = "- " + clean[index]
		}
		writeTicketSection(ticket, heading, strings.Join(clean, "\n"))
	}
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
