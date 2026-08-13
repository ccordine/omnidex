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

const maxScrumCardStateActionBodyBytes int64 = 8 * 1024

type scrumCardMoveRequest struct {
	Column       string `json:"column"`
	BeforeCardID string `json:"before_card_id"`
}

type scrumCardDoneRequest struct{}

func decodeScrumCardMoveRequest(w http.ResponseWriter, r *http.Request) (scrumCardMoveRequest, error) {
	var request scrumCardMoveRequest
	if err := decodeExactScrumCardStateAction(w, r, &request, "Scrum card move"); err != nil {
		return scrumCardMoveRequest{}, err
	}
	request.Column = strings.TrimSpace(request.Column)
	request.BeforeCardID = strings.TrimSpace(request.BeforeCardID)
	if request.Column == "" {
		return scrumCardMoveRequest{}, fmt.Errorf("Scrum card move column is required")
	}
	if strings.ContainsRune(request.Column, '\x00') || strings.ContainsRune(request.BeforeCardID, '\x00') {
		return scrumCardMoveRequest{}, fmt.Errorf("Scrum card move fields must not contain NUL")
	}
	return request, nil
}

func decodeScrumCardDoneRequest(w http.ResponseWriter, r *http.Request) error {
	var request scrumCardDoneRequest
	return decodeExactScrumCardStateAction(w, r, &request, "Scrum card done")
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
			return fmt.Errorf("%s exceeds the %d-byte transport bound", name, maxScrumCardStateActionBodyBytes)
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
	if strings.Contains(err.Error(), "transport bound") {
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}
