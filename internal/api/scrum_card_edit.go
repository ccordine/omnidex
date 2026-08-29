package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/queue"
)

const maxScrumCardEditBodyBytes int64 = 256 * 1024

type scrumCardEditableField[T any] struct {
	Present bool
	Value   T
}

func (field *scrumCardEditableField[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("editable Scrum card fields must not be null")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value T
	if err := exactjson.ValidateExactFields(data, value, "editable Scrum card field"); err != nil {
		return err
	}
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if err := requireJSONEOF(decoder, "editable Scrum card field"); err != nil {
		return err
	}
	field.Present = true
	field.Value = value
	return nil
}

func editableScrumCardField[T any](value T) scrumCardEditableField[T] {
	return scrumCardEditableField[T]{Present: true, Value: value}
}

type scrumCardEditRequest struct {
	ExpectedUpdatedAt requiredScrumCardRevision        `json:"expected_updated_at"`
	Title             scrumCardEditableField[string]   `json:"title"`
	Description       scrumCardEditableField[string]   `json:"description"`
	RefFiles          scrumCardEditableField[[]string] `json:"ref_files"`
	CardTicket        scrumCardEditableField[string]   `json:"card_ticket"`
	CardPrompt        scrumCardEditableField[string]   `json:"card_prompt"`
	Tags              scrumCardEditableField[[]string] `json:"tags"`
}

func (edit scrumCardEditRequest) hasEditableField() bool {
	return edit.Title.Present || edit.Description.Present ||
		edit.RefFiles.Present || edit.CardTicket.Present ||
		edit.CardPrompt.Present || edit.Tags.Present
}

func (edit scrumCardEditRequest) validate() error {
	if !edit.ExpectedUpdatedAt.Present {
		return fmt.Errorf("Scrum card editable patch expected_updated_at is required")
	}
	if edit.Title.Present && strings.TrimSpace(edit.Title.Value) == "" {
		return fmt.Errorf("editable Scrum card title must not be blank")
	}
	return nil
}

func (edit scrumCardEditRequest) repositoryPatch() queue.ScrumCardRevisionPatch {
	var patch queue.ScrumCardRevisionPatch
	if edit.Title.Present {
		value := strings.TrimSpace(edit.Title.Value)
		patch.Title = &value
	}
	if edit.Description.Present {
		value := edit.Description.Value
		patch.Description = &value
	}
	if edit.RefFiles.Present {
		value := append([]string(nil), edit.RefFiles.Value...)
		patch.RefFiles = &value
	}
	if edit.CardTicket.Present {
		value := edit.CardTicket.Value
		patch.CardTicket = &value
	}
	if edit.CardPrompt.Present {
		value := edit.CardPrompt.Value
		patch.CardPrompt = &value
	}
	if edit.Tags.Present {
		value := normalizeScrumCardTags(edit.Tags.Value)
		patch.Tags = &value
	}
	return patch
}

func normalizeScrumCardTags(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		tag := canonicalScrumTag(value)
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	return normalized
}

func canonicalScrumTag(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), "-"))
}

type scrumCardEditBodyError struct {
	status int
	err    error
}

func (bodyError scrumCardEditBodyError) Error() string { return bodyError.err.Error() }

func decodeScrumCardEditRequest(
	w http.ResponseWriter,
	r *http.Request,
) (scrumCardEditRequest, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxScrumCardEditBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return scrumCardEditRequest{}, newScrumCardEditBodyError(err)
	}
	if !utf8.Valid(raw) {
		return scrumCardEditRequest{}, newScrumCardEditBodyError(fmt.Errorf("Scrum card editable patch must be valid UTF-8"))
	}
	if err := exactjson.ValidateObject(raw, scrumCardEditRequest{}, "Scrum card editable patch"); err != nil {
		return scrumCardEditRequest{}, newScrumCardEditBodyError(err)
	}
	if err := validateScrumCardEditNulls(raw); err != nil {
		return scrumCardEditRequest{}, newScrumCardEditBodyError(err)
	}
	if err := validateScrumCardEditText(raw); err != nil {
		return scrumCardEditRequest{}, newScrumCardEditBodyError(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var edit scrumCardEditRequest
	if err := decoder.Decode(&edit); err != nil {
		return scrumCardEditRequest{}, newScrumCardEditBodyError(err)
	}
	if err := requireJSONEOF(decoder, "Scrum card editable patch"); err != nil {
		return scrumCardEditRequest{}, newScrumCardEditBodyError(err)
	}
	if !edit.hasEditableField() {
		return scrumCardEditRequest{}, scrumCardEditBodyError{
			status: http.StatusBadRequest,
			err:    fmt.Errorf("Scrum card editable patch requires at least one editable field"),
		}
	}
	if err := edit.validate(); err != nil {
		return scrumCardEditRequest{}, scrumCardEditBodyError{status: http.StatusBadRequest, err: err}
	}
	return edit, nil
}

func newScrumCardEditBodyError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return scrumCardEditBodyError{
			status: http.StatusRequestEntityTooLarge,
			err: fmt.Errorf(
				"Scrum card editable patch exceeds the %d-byte transport bound",
				maxScrumCardEditBodyBytes,
			),
		}
	}
	return scrumCardEditBodyError{
		status: http.StatusBadRequest,
		err:    fmt.Errorf("invalid Scrum card editable patch JSON: %w", err),
	}
}

func writeScrumCardEditBodyError(w http.ResponseWriter, err error) {
	var bodyError scrumCardEditBodyError
	if errors.As(err, &bodyError) {
		writeError(w, bodyError.status, bodyError.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}
