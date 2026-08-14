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
)

const maxBrowseMkdirBodyBytes int64 = 8 * 1024

type browseMkdirRequest struct {
	Parent string `json:"parent"`
	Name   string `json:"name"`
}

func decodeBrowseMkdirRequest(w http.ResponseWriter, r *http.Request) (browseMkdirRequest, error) {
	if r == nil || r.Body == nil {
		return browseMkdirRequest{}, fmt.Errorf("directory creation body is required")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBrowseMkdirBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return browseMkdirRequest{}, fmt.Errorf("read directory creation request: %w", err)
	}
	if !utf8.Valid(raw) {
		return browseMkdirRequest{}, fmt.Errorf("directory creation request must be valid UTF-8")
	}
	if err := exactjson.ValidateObject(raw, browseMkdirRequest{}, "directory creation request"); err != nil {
		return browseMkdirRequest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request browseMkdirRequest
	if err := decoder.Decode(&request); err != nil {
		return browseMkdirRequest{}, fmt.Errorf("decode directory creation request: %w", err)
	}
	if err := requireJSONEOF(decoder, "directory creation request"); err != nil {
		return browseMkdirRequest{}, err
	}
	if !utf8.ValidString(request.Parent) || len(request.Parent) > 4096 || strings.ContainsRune(request.Parent, '\x00') ||
		request.Parent != strings.TrimSpace(request.Parent) {
		return browseMkdirRequest{}, fmt.Errorf("directory creation parent must be one canonical UTF-8 path of at most 4096 bytes")
	}
	if !utf8.ValidString(request.Name) || len(request.Name) > 255 || strings.ContainsRune(request.Name, '\x00') ||
		request.Name == "" || request.Name != strings.TrimSpace(request.Name) || request.Name == "." || request.Name == ".." ||
		strings.ContainsAny(request.Name, `/\`) {
		return browseMkdirRequest{}, fmt.Errorf("directory creation name must be one canonical path segment of at most 255 bytes")
	}
	return request, nil
}

func browseMkdirRequestStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}
