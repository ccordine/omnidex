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
	"github.com/gryph/omnidex/internal/modelconfig"
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
	Title       scrumCardEditableField[string]          `json:"title"`
	Description scrumCardEditableField[string]          `json:"description"`
	RefFiles    scrumCardEditableField[[]string]        `json:"ref_files"`
	ModelConfig scrumCardEditableField[json.RawMessage] `json:"model_config"`
	CardTicket  scrumCardEditableField[string]          `json:"card_ticket"`
	CardPrompt  scrumCardEditableField[string]          `json:"card_prompt"`
	RecipeID    scrumCardEditableField[string]          `json:"recipe_id"`
	Recipe      scrumCardEditableField[json.RawMessage] `json:"recipe"`
	Tags        scrumCardEditableField[[]string]        `json:"tags"`
}

func (edit scrumCardEditRequest) hasEditableField() bool {
	return edit.Title.Present || edit.Description.Present ||
		edit.RefFiles.Present || edit.ModelConfig.Present || edit.CardTicket.Present ||
		edit.CardPrompt.Present || edit.RecipeID.Present || edit.Recipe.Present ||
		edit.Tags.Present
}

func (edit scrumCardEditRequest) validate() error {
	if edit.Title.Present && strings.TrimSpace(edit.Title.Value) == "" {
		return fmt.Errorf("editable Scrum card title must not be blank")
	}
	if edit.ModelConfig.Present {
		if _, err := modelconfig.FromJSON(edit.ModelConfig.Value); err != nil {
			return fmt.Errorf("editable Scrum card model_config is invalid: %w", err)
		}
	}
	if edit.Recipe.Present {
		var object map[string]json.RawMessage
		decoder := json.NewDecoder(bytes.NewReader(edit.Recipe.Value))
		if err := decoder.Decode(&object); err != nil {
			return fmt.Errorf("editable Scrum card recipe must be a JSON object: %w", err)
		}
		if err := requireJSONEOF(decoder, "editable Scrum card recipe"); err != nil {
			return err
		}
		if object == nil {
			return fmt.Errorf("editable Scrum card recipe must be a JSON object")
		}
	}
	return nil
}

func (edit scrumCardEditRequest) repositoryPatch() (map[string]any, error) {
	patch := make(map[string]any, 11)
	if edit.Title.Present {
		patch["title"] = strings.TrimSpace(edit.Title.Value)
	}
	if edit.Description.Present {
		patch["description"] = edit.Description.Value
	}
	if edit.RefFiles.Present {
		encoded, err := json.Marshal(edit.RefFiles.Value)
		if err != nil {
			return nil, fmt.Errorf("encode editable Scrum card reference files: %w", err)
		}
		patch["ref_files"] = json.RawMessage(encoded)
	}
	if edit.ModelConfig.Present {
		patch["model_config"] = edit.ModelConfig.Value
	}
	if edit.CardTicket.Present {
		patch["card_ticket"] = edit.CardTicket.Value
	}
	if edit.CardPrompt.Present {
		patch["card_prompt"] = edit.CardPrompt.Value
	}
	if edit.RecipeID.Present {
		patch["recipe_id"] = strings.TrimSpace(edit.RecipeID.Value)
	}
	if edit.Recipe.Present {
		patch["recipe"] = edit.Recipe.Value
	}
	if edit.Tags.Present {
		encoded, err := json.Marshal(edit.Tags.Value)
		if err != nil {
			return nil, fmt.Errorf("encode editable Scrum card tags: %w", err)
		}
		patch["tags"] = json.RawMessage(encoded)
	}
	return patch, nil
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
