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
)

const (
	maxProjectMutationBodyBytes int64 = 32 * 1024
	maxProjectNameBytes               = 256
	maxProjectLocationBytes           = 4096
	maxProjectDescriptionBytes        = 16 * 1024
)

type projectTextField struct {
	Present bool
	Value   string
}

func (field *projectTextField) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("project text fields must not be null")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("project text fields must be strings: %w", err)
	}
	field.Present = true
	field.Value = value
	return nil
}

type projectModelConfigField struct {
	Present bool
	Value   json.RawMessage
}

func (field *projectModelConfigField) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("project model_config must be one JSON object")
	}
	field.Present = true
	field.Value = append(field.Value[:0], raw...)
	return nil
}

type projectCreateBody struct {
	Name        projectTextField `json:"name"`
	Location    projectTextField `json:"location"`
	Description projectTextField `json:"description"`
}

type projectCreateRequest struct {
	Name        string
	Location    string
	Description string
}

type projectPatchBody struct {
	ExpectedUpdatedAt projectRevisionField    `json:"expected_updated_at"`
	Name              projectTextField        `json:"name"`
	Location          projectTextField        `json:"location"`
	Description       projectTextField        `json:"description"`
	ModelConfig       projectModelConfigField `json:"model_config"`
}

type projectPatchRequest struct {
	ExpectedUpdatedAt time.Time
	Name              projectTextField
	Location          projectTextField
	Description       projectTextField
	ModelConfig       projectModelConfigField
}

type projectDeleteBody struct {
	ExpectedUpdatedAt projectRevisionField `json:"expected_updated_at"`
}

type projectRevisionField struct {
	Present bool
	Value   time.Time
}

func (field *projectRevisionField) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("project expected_updated_at must not be null")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("project expected_updated_at must be a string: %w", err)
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Location() != time.UTC ||
		!parsed.Equal(parsed.Truncate(time.Microsecond)) || parsed.Format(time.RFC3339Nano) != value {
		return fmt.Errorf("project expected_updated_at must be canonical UTC microsecond time")
	}
	field.Present, field.Value = true, parsed
	return nil
}

func decodeProjectCreateRequest(w http.ResponseWriter, r *http.Request) (projectCreateRequest, error) {
	raw, err := readExactProjectBody(w, r, "project create")
	if err != nil {
		return projectCreateRequest{}, err
	}
	if err := exactjson.ValidateObject(raw, projectCreateBody{}, "project create request"); err != nil {
		return projectCreateRequest{}, err
	}
	var body projectCreateBody
	if err := decodeExactProjectJSON(raw, "project create request", &body); err != nil {
		return projectCreateRequest{}, err
	}
	if !body.Name.Present || strings.TrimSpace(body.Name.Value) == "" {
		return projectCreateRequest{}, fmt.Errorf("project create name is required")
	}
	if !body.Location.Present || strings.TrimSpace(body.Location.Value) == "" {
		return projectCreateRequest{}, fmt.Errorf("project create location is required")
	}
	if err := validateProjectText("name", body.Name.Value, maxProjectNameBytes, false); err != nil {
		return projectCreateRequest{}, err
	}
	if err := validateProjectText("location", body.Location.Value, maxProjectLocationBytes, false); err != nil {
		return projectCreateRequest{}, err
	}
	description := ""
	if body.Description.Present {
		description = body.Description.Value
		if err := validateProjectText("description", description, maxProjectDescriptionBytes, true); err != nil {
			return projectCreateRequest{}, err
		}
	}
	return projectCreateRequest{
		Name: body.Name.Value, Location: body.Location.Value, Description: description,
	}, nil
}

func decodeProjectPatchRequest(w http.ResponseWriter, r *http.Request) (projectPatchRequest, error) {
	raw, err := readExactProjectBody(w, r, "project patch")
	if err != nil {
		return projectPatchRequest{}, err
	}
	if err := exactjson.ValidateObject(raw, projectPatchBody{}, "project patch request"); err != nil {
		return projectPatchRequest{}, err
	}
	var body projectPatchBody
	if err := decodeExactProjectJSON(raw, "project patch request", &body); err != nil {
		return projectPatchRequest{}, err
	}
	if !body.ExpectedUpdatedAt.Present {
		return projectPatchRequest{}, fmt.Errorf("project patch expected_updated_at is required")
	}
	if !body.Name.Present && !body.Location.Present && !body.Description.Present && !body.ModelConfig.Present {
		return projectPatchRequest{}, fmt.Errorf("project patch requires one typed field")
	}
	if body.Name.Present {
		if strings.TrimSpace(body.Name.Value) == "" {
			return projectPatchRequest{}, fmt.Errorf("project patch name must not be blank")
		}
		if err := validateProjectText("name", body.Name.Value, maxProjectNameBytes, false); err != nil {
			return projectPatchRequest{}, err
		}
	}
	if body.Location.Present {
		if strings.TrimSpace(body.Location.Value) == "" {
			return projectPatchRequest{}, fmt.Errorf("project patch location must not be blank")
		}
		if err := validateProjectText("location", body.Location.Value, maxProjectLocationBytes, false); err != nil {
			return projectPatchRequest{}, err
		}
	}
	if body.Description.Present {
		if err := validateProjectText("description", body.Description.Value, maxProjectDescriptionBytes, true); err != nil {
			return projectPatchRequest{}, err
		}
	}
	if body.ModelConfig.Present {
		if err := validateExactProjectModelConfig(body.ModelConfig.Value); err != nil {
			return projectPatchRequest{}, err
		}
	}
	return projectPatchRequest{
		ExpectedUpdatedAt: body.ExpectedUpdatedAt.Value,
		Name:              body.Name, Location: body.Location, Description: body.Description, ModelConfig: body.ModelConfig,
	}, nil
}

func decodeProjectDeleteRequest(w http.ResponseWriter, r *http.Request) (time.Time, error) {
	raw, err := readExactProjectBody(w, r, "project delete")
	if err != nil {
		return time.Time{}, err
	}
	if err := exactjson.ValidateObject(raw, projectDeleteBody{}, "project delete request"); err != nil {
		return time.Time{}, err
	}
	var body projectDeleteBody
	if err := decodeExactProjectJSON(raw, "project delete request", &body); err != nil {
		return time.Time{}, err
	}
	if !body.ExpectedUpdatedAt.Present {
		return time.Time{}, fmt.Errorf("project delete expected_updated_at is required")
	}
	return body.ExpectedUpdatedAt.Value, nil
}

func validateExactProjectModelConfig(raw json.RawMessage) error {
	if _, err := modelConfigPatchFromRequest(raw); err != nil {
		return err
	}
	return nil
}

func readExactProjectBody(w http.ResponseWriter, r *http.Request, name string) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, fmt.Errorf("%s body is required", name)
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxProjectMutationBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, fmt.Errorf("%s exceeds the %d-byte transport bound: %w", name, maxProjectMutationBodyBytes, err)
		}
		return nil, fmt.Errorf("read %s request: %w", name, err)
	}
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("%s request must be valid UTF-8", name)
	}
	return raw, nil
}

func decodeExactProjectJSON(raw []byte, name string, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return requireJSONEOF(decoder, name)
}

func validateProjectText(name, value string, maxBytes int, allowBlank bool) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("project %s must be valid UTF-8 without NUL", name)
	}
	if len(value) > maxBytes {
		return fmt.Errorf("project %s exceeds the %d-byte bound", name, maxBytes)
	}
	if !allowBlank && strings.TrimSpace(value) == "" {
		return fmt.Errorf("project %s must not be blank", name)
	}
	return nil
}

func projectRequestErrorStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}
