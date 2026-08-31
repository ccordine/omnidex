package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

const maxGenericCodingEnqueueBodyBytes int64 = 256 * 1024

type enqueueRequest struct {
	Instruction string                 `json:"instruction"`
	Pipeline    string                 `json:"pipeline"`
	Metadata    *genericCodingMetadata `json:"metadata"`
}

type genericCodingMetadata struct {
	ClientCWD string `json:"client_cwd"`
	SessionID string `json:"session_id,omitempty"`
}

type genericCodingRuntimeMetadata struct {
	ClientCWD   string             `json:"client_cwd"`
	SessionID   string             `json:"session_id,omitempty"`
	ModelConfig modelconfig.Config `json:"model_config"`
}

func (s *Server) genericCodingRuntimeMetadata(metadata genericCodingMetadata) ([]byte, error) {
	if err := validateGenericCodingMetadata(metadata); err != nil {
		return nil, err
	}
	modelSnapshot := s.runtimeModelConfig()
	return json.Marshal(genericCodingRuntimeMetadata{
		ClientCWD: metadata.ClientCWD, SessionID: metadata.SessionID,
		ModelConfig: modelSnapshot,
	})
}

func decodeGenericCodingEnqueue(w http.ResponseWriter, r *http.Request) (enqueueRequest, error) {
	if r == nil || r.Body == nil {
		return enqueueRequest{}, fmt.Errorf("coding enqueue request body is required")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxGenericCodingEnqueueBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return enqueueRequest{}, fmt.Errorf("read coding enqueue request: %w", err)
	}
	if !utf8.Valid(raw) {
		return enqueueRequest{}, fmt.Errorf("coding enqueue request must be valid UTF-8")
	}
	if err := exactjson.ValidateObject(raw, enqueueRequest{}, "coding enqueue request"); err != nil {
		return enqueueRequest{}, err
	}
	var request enqueueRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return enqueueRequest{}, fmt.Errorf("decode coding enqueue request: %w", err)
	}
	if err := requireJSONEOF(decoder, "coding enqueue request"); err != nil {
		return enqueueRequest{}, err
	}
	if request.Pipeline != model.PipelineCoding {
		return enqueueRequest{}, validateGenericJobPipeline(request.Pipeline)
	}
	if request.Metadata == nil {
		return enqueueRequest{}, fmt.Errorf("coding enqueue metadata must be one JSON object")
	}
	if err := validateGenericCodingMetadata(*request.Metadata); err != nil {
		return enqueueRequest{}, err
	}
	if _, err := requireFreeFormAuthority(request.Instruction, "instruction"); err != nil {
		return enqueueRequest{}, err
	}
	return request, nil
}

func validateGenericCodingMetadata(metadata genericCodingMetadata) error {
	if value := metadata.ClientCWD; value == "" || value != strings.TrimSpace(value) ||
		strings.ContainsRune(value, '\x00') || len(value) > 4096 ||
		!filepath.IsAbs(value) || filepath.Clean(value) != value {
		return fmt.Errorf("coding enqueue metadata client_cwd must be one canonical absolute workspace root")
	}
	if metadata.SessionID != "" && (metadata.SessionID != strings.TrimSpace(metadata.SessionID) ||
		strings.ContainsRune(metadata.SessionID, '\x00') || len(metadata.SessionID) > 256) {
		return fmt.Errorf("coding enqueue session_id must be a canonical string of at most 256 bytes")
	}
	return nil
}

func genericCodingEnqueueStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}
