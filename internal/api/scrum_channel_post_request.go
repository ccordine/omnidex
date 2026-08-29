package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

// The decoded user turn is bounded to 4 KiB. The transport bound also permits
// the largest JSON-escaped representation plus the exact lifecycle identity.
const maxScrumChannelPostBodyBytes int64 = 32 * 1024

type scrumChannelPostRequest struct {
	OperationID queue.LifecycleOperationID `json:"operation_id"`
	Message     string                     `json:"message"`
}

func decodeScrumChannelPostRequest(
	w http.ResponseWriter,
	r *http.Request,
) (scrumChannelPostRequest, error) {
	if r == nil || r.Body == nil {
		return scrumChannelPostRequest{}, fmt.Errorf("Scrum channel POST body is required")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxScrumChannelPostBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return scrumChannelPostRequest{}, fmt.Errorf(
				"Scrum channel POST exceeds the %d-byte transport bound: %w",
				maxScrumChannelPostBodyBytes, err,
			)
		}
		return scrumChannelPostRequest{}, fmt.Errorf("read Scrum channel POST: %w", err)
	}
	if !utf8.Valid(raw) {
		return scrumChannelPostRequest{}, fmt.Errorf("Scrum channel POST must be valid UTF-8")
	}
	if err := exactjson.ValidateObject(raw, scrumChannelPostRequest{}, "Scrum channel POST"); err != nil {
		return scrumChannelPostRequest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request scrumChannelPostRequest
	if err := decoder.Decode(&request); err != nil {
		return scrumChannelPostRequest{}, fmt.Errorf("decode Scrum channel POST: %w", err)
	}
	if err := requireJSONEOF(decoder, "Scrum channel POST"); err != nil {
		return scrumChannelPostRequest{}, err
	}
	operationID, err := queue.ParseLifecycleOperationID(string(request.OperationID))
	if err != nil {
		return scrumChannelPostRequest{}, err
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, request.Message); err != nil {
		return scrumChannelPostRequest{}, fmt.Errorf("Scrum channel message: %w", err)
	}
	request.OperationID = operationID
	return request, nil
}

func scrumChannelPostBodyStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}
