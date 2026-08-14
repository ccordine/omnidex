package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	maxScrumAutoWorkBodyBytes     int64 = 8 * 1024
	maxScrumCardCreateBodyBytes   int64 = 64 * 1024
	maxScrumCardTitleBytes              = 1024
	maxScrumCardDescriptionBytes        = 32 * 1024
	maxScrumMutationRawQueryBytes       = 4 * 1024
)

var errRemovedScrumMutationAuthority = errors.New("removed Scrum mutation authority")

type requiredScrumBool struct {
	Present bool
	Value   bool
}

func (field *requiredScrumBool) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("Scrum boolean fields must not be null")
	}
	if err := json.Unmarshal(raw, &field.Value); err != nil {
		return fmt.Errorf("Scrum boolean fields must be booleans: %w", err)
	}
	field.Present = true
	return nil
}

type requiredScrumStrings struct {
	Present bool
	Value   []string
}

func (field *requiredScrumStrings) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("Scrum string-list fields must not be null")
	}
	if err := json.Unmarshal(raw, &field.Value); err != nil {
		return fmt.Errorf("Scrum string-list fields must be arrays of strings: %w", err)
	}
	field.Present = true
	return nil
}

type scrumAutoWorkBody struct {
	Enabled       requiredScrumBool    `json:"enabled"`
	SourceColumns requiredScrumStrings `json:"source_columns"`
}

type scrumAutoWorkRequestBody struct {
	AutoWork            json.RawMessage `json:"auto_work"`
	RemovedAutoReview   json.RawMessage `json:"auto_review"`
	RemovedCreateTicket json.RawMessage `json:"create_ticket"`
}

func decodeScrumAutoWorkRequest(w http.ResponseWriter, r *http.Request) (ScrumAutoWorkConfig, error) {
	raw, err := readExactScrumMutationBody(w, r, "Scrum auto-work request", maxScrumAutoWorkBodyBytes)
	if err != nil {
		return ScrumAutoWorkConfig{}, err
	}
	if err := exactjson.ValidateObject(raw, scrumAutoWorkRequestBody{}, "Scrum auto-work request"); err != nil {
		return ScrumAutoWorkConfig{}, err
	}
	var body scrumAutoWorkRequestBody
	if err := decodeExactScrumMutationJSON(raw, "Scrum auto-work request", &body); err != nil {
		return ScrumAutoWorkConfig{}, err
	}
	if len(body.RemovedCreateTicket) > 0 {
		return ScrumAutoWorkConfig{}, fmt.Errorf("removed Scrum create-ticket automation has no compatibility path: %w", errRemovedScrumMutationAuthority)
	}
	if len(body.RemovedAutoReview) > 0 {
		return ScrumAutoWorkConfig{}, fmt.Errorf("removed Scrum auto-review automation has no compatibility path: %w", errRemovedScrumMutationAuthority)
	}
	if len(body.AutoWork) == 0 {
		return ScrumAutoWorkConfig{}, fmt.Errorf("Scrum auto-work request auto_work is required")
	}
	if bytes.Equal(bytes.TrimSpace(body.AutoWork), []byte("null")) {
		return ScrumAutoWorkConfig{}, fmt.Errorf("Scrum auto-work request auto_work must be one object")
	}
	if err := exactjson.ValidateObject(body.AutoWork, scrumAutoWorkBody{}, "Scrum auto-work config"); err != nil {
		return ScrumAutoWorkConfig{}, err
	}
	var config scrumAutoWorkBody
	if err := decodeExactScrumMutationJSON(body.AutoWork, "Scrum auto-work config", &config); err != nil {
		return ScrumAutoWorkConfig{}, err
	}
	if !config.Enabled.Present {
		return ScrumAutoWorkConfig{}, fmt.Errorf("Scrum auto-work enabled is required")
	}
	if !config.SourceColumns.Present {
		return ScrumAutoWorkConfig{}, fmt.Errorf("Scrum auto-work source_columns is required")
	}
	return validateScrumAutoWorkConfig(ScrumAutoWorkConfig{
		Enabled: config.Enabled.Value, SourceColumns: config.SourceColumns.Value,
	})
}

type requiredScrumText struct {
	Present bool
	Value   string
}

func (field *requiredScrumText) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("Scrum text fields must not be null")
	}
	if err := json.Unmarshal(raw, &field.Value); err != nil {
		return fmt.Errorf("Scrum text fields must be strings: %w", err)
	}
	field.Present = true
	return nil
}

type scrumCardCreateBody struct {
	Title                     requiredScrumText `json:"title"`
	Description               requiredScrumText `json:"description"`
	Column                    requiredScrumText `json:"column"`
	RemovedCreateTicket       json.RawMessage   `json:"create_ticket"`
	RemovedCreateTicketConfig json.RawMessage   `json:"create_ticket_config"`
}

type scrumCardCreateRequest struct {
	Title       string
	Description string
	Column      queue.ScrumCardColumn
}

func decodeScrumCardCreateRequest(w http.ResponseWriter, r *http.Request) (scrumCardCreateRequest, error) {
	raw, err := readExactScrumMutationBody(w, r, "Scrum card create request", maxScrumCardCreateBodyBytes)
	if err != nil {
		return scrumCardCreateRequest{}, err
	}
	if err := exactjson.ValidateObject(raw, scrumCardCreateBody{}, "Scrum card create request"); err != nil {
		return scrumCardCreateRequest{}, err
	}
	var body scrumCardCreateBody
	if err := decodeExactScrumMutationJSON(raw, "Scrum card create request", &body); err != nil {
		return scrumCardCreateRequest{}, err
	}
	if len(body.RemovedCreateTicket) > 0 || len(body.RemovedCreateTicketConfig) > 0 {
		return scrumCardCreateRequest{}, fmt.Errorf("removed Scrum card ticket generation has no compatibility path: %w", errRemovedScrumMutationAuthority)
	}
	if !body.Title.Present {
		return scrumCardCreateRequest{}, fmt.Errorf("Scrum card create title is required")
	}
	if strings.TrimSpace(body.Title.Value) == "" {
		return scrumCardCreateRequest{}, fmt.Errorf("Scrum card create title must not be blank")
	}
	if !body.Description.Present {
		return scrumCardCreateRequest{}, fmt.Errorf("Scrum card create description is required")
	}
	if !body.Column.Present {
		return scrumCardCreateRequest{}, fmt.Errorf("Scrum card create column is required")
	}
	for name, field := range map[string]struct {
		value string
		limit int
	}{
		"title":       {body.Title.Value, maxScrumCardTitleBytes},
		"description": {body.Description.Value, maxScrumCardDescriptionBytes},
	} {
		if !utf8.ValidString(field.value) || strings.ContainsRune(field.value, '\x00') {
			return scrumCardCreateRequest{}, fmt.Errorf("Scrum card create %s must be valid UTF-8 without NUL", name)
		}
		if len(field.value) > field.limit {
			return scrumCardCreateRequest{}, fmt.Errorf("Scrum card create %s exceeds the %d-byte bound", name, field.limit)
		}
	}
	column, err := queue.ParseScrumCardColumn(body.Column.Value)
	if err != nil {
		return scrumCardCreateRequest{}, err
	}
	return scrumCardCreateRequest{
		Title: strings.TrimSpace(body.Title.Value), Description: body.Description.Value, Column: column,
	}, nil
}

func decodeScrumMutationProjectID(r *http.Request, name string) (int64, error) {
	if r == nil || r.URL == nil {
		return 0, fmt.Errorf("%s request URL is required", name)
	}
	if len(r.URL.RawQuery) > maxScrumMutationRawQueryBytes {
		return 0, fmt.Errorf("%s query exceeds the %d-byte bound", name, maxScrumMutationRawQueryBytes)
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return 0, fmt.Errorf("decode %s query: %w", name, err)
	}
	for key, items := range values {
		if key != "project_id" {
			return 0, fmt.Errorf("%s has unknown query field %q", name, key)
		}
		if len(items) != 1 {
			return 0, fmt.Errorf("%s query field %q must occur exactly once", name, key)
		}
	}
	rawProjectID, present := oneQueryValue(values, "project_id")
	if !present {
		return 0, fmt.Errorf("%s requires project_id", name)
	}
	projectID, err := strconv.ParseInt(rawProjectID, 10, 64)
	if err != nil || projectID <= 0 || strconv.FormatInt(projectID, 10) != rawProjectID {
		return 0, fmt.Errorf("%s project_id must be one canonical positive integer", name)
	}
	return projectID, nil
}

func readExactScrumMutationBody(w http.ResponseWriter, r *http.Request, name string, maxBytes int64) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, fmt.Errorf("%s body is required", name)
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, fmt.Errorf("%s exceeds the %d-byte transport bound: %w", name, maxBytes, err)
		}
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("%s must be valid UTF-8", name)
	}
	return raw, nil
}

func decodeExactScrumMutationJSON(raw []byte, name string, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return requireJSONEOF(decoder, name)
}

func writeScrumMutationBodyError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}
