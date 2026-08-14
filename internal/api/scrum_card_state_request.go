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

const maxScrumCardStateActionBodyBytes int64 = 8 * 1024

type scrumCardMoveRequest struct {
	Column            queue.ScrumCardColumn     `json:"column"`
	BeforeCardID      string                    `json:"before_card_id"`
	ExpectedUpdatedAt requiredScrumCardRevision `json:"expected_updated_at"`
}

type scrumCardDoneRequest struct {
	ExpectedUpdatedAt requiredScrumCardRevision `json:"expected_updated_at"`
}

type requiredScrumCardRevision struct {
	Present bool
	Value   time.Time
}

func (revision *requiredScrumCardRevision) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("Scrum card expected_updated_at must be a canonical UTC timestamp")
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return fmt.Errorf("Scrum card expected_updated_at must be a string: %w", err)
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil || text != parsed.UTC().Format(time.RFC3339Nano) {
		return fmt.Errorf("Scrum card expected_updated_at must be a canonical UTC timestamp")
	}
	revision.Present = true
	revision.Value = parsed
	return nil
}

func decodeScrumCardMoveRequest(w http.ResponseWriter, r *http.Request) (scrumCardMoveRequest, error) {
	var request scrumCardMoveRequest
	if err := decodeExactScrumCardStateAction(w, r, &request, "Scrum card move"); err != nil {
		return scrumCardMoveRequest{}, err
	}
	if _, err := queue.ParseScrumCardColumn(string(request.Column)); err != nil {
		return scrumCardMoveRequest{}, err
	}
	if !request.ExpectedUpdatedAt.Present {
		return scrumCardMoveRequest{}, fmt.Errorf("Scrum card move expected_updated_at is required")
	}
	if err := validateOptionalScrumCardID(request.BeforeCardID, "Scrum card move before_card_id"); err != nil {
		return scrumCardMoveRequest{}, err
	}
	if request.BeforeCardID != strings.TrimSpace(request.BeforeCardID) {
		return scrumCardMoveRequest{}, fmt.Errorf("Scrum card move before_card_id must be canonical")
	}
	if strings.ContainsRune(string(request.Column), '\x00') || strings.ContainsRune(request.BeforeCardID, '\x00') {
		return scrumCardMoveRequest{}, fmt.Errorf("Scrum card move fields must not contain NUL")
	}
	return request, nil
}

func validateOptionalScrumCardID(value, field string) error {
	if value == "" {
		return nil
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') ||
		value != strings.TrimSpace(value) || len(value) > queue.MaxScrumCardIDBytes {
		return fmt.Errorf("%s must be one canonical card ID bounded to %d bytes", field, queue.MaxScrumCardIDBytes)
	}
	return nil
}

func decodeScrumCardDoneRequest(w http.ResponseWriter, r *http.Request) (scrumCardDoneRequest, error) {
	var request scrumCardDoneRequest
	if err := decodeExactScrumCardStateAction(w, r, &request, "Scrum card done"); err != nil {
		return scrumCardDoneRequest{}, err
	}
	if !request.ExpectedUpdatedAt.Present {
		return scrumCardDoneRequest{}, fmt.Errorf("Scrum card done expected_updated_at is required")
	}
	return request, nil
}

func decodeExactScrumCardStateAction(
	w http.ResponseWriter,
	r *http.Request,
	target any,
	name string,
) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxScrumCardStateActionBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return fmt.Errorf("%s exceeds the %d-byte transport bound: %w", name, maxScrumCardStateActionBodyBytes, err)
		}
		return fmt.Errorf("read %s request: %w", name, err)
	}
	if !utf8.Valid(raw) {
		return fmt.Errorf("%s request must be valid UTF-8", name)
	}
	if err := exactjson.ValidateObject(raw, target, name+" request"); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s request: %w", name, err)
	}
	return requireJSONEOF(decoder, name+" request")
}

func writeScrumCardStateBodyError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}
