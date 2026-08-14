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

const (
	maxLifecycleControlTextBytes = 64 * 1024
	// JSON escaping can expand one accepted lifecycle-text byte to six bytes.
	maxLifecycleControlBodyBytes int64 = 512 * 1024
)

func decodeLifecycleFeedbackRequest(w http.ResponseWriter, r *http.Request) (feedbackRequest, error) {
	var request feedbackRequest
	if err := decodeLifecycleControlObject(w, r, "lifecycle feedback request", &request); err != nil {
		return feedbackRequest{}, err
	}
	operationID, err := queue.ParseLifecycleOperationID(string(request.OperationID))
	if err != nil {
		return feedbackRequest{}, err
	}
	if err := validateLifecycleControlText(request.Feedback, "feedback"); err != nil {
		return feedbackRequest{}, err
	}
	request.OperationID = operationID
	return request, nil
}

func decodeLifecycleCancelRequest(w http.ResponseWriter, r *http.Request) (cancelRequest, error) {
	var request cancelRequest
	if err := decodeLifecycleControlObject(w, r, "lifecycle cancellation request", &request); err != nil {
		return cancelRequest{}, err
	}
	operationID, err := queue.ParseLifecycleOperationID(string(request.OperationID))
	if err != nil {
		return cancelRequest{}, err
	}
	if err := validateLifecycleControlText(request.Reason, "cancel reason"); err != nil {
		return cancelRequest{}, err
	}
	request.OperationID = operationID
	return request, nil
}

func decodeLifecycleControlObject(
	w http.ResponseWriter,
	r *http.Request,
	name string,
	destination any,
) error {
	if r == nil || r.Body == nil {
		return fmt.Errorf("%s body is required", name)
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxLifecycleControlBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return fmt.Errorf("%s exceeds the %d-byte transport bound: %w", name, maxLifecycleControlBodyBytes, err)
		}
		return fmt.Errorf("read %s: %w", name, err)
	}
	if !utf8.Valid(raw) {
		return fmt.Errorf("%s must be valid UTF-8", name)
	}
	if err := exactjson.ValidateObject(raw, destination, name); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return requireJSONEOF(decoder, name)
}

func validateLifecycleControlText(value, name string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", name)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s must not contain NUL", name)
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > maxLifecycleControlTextBytes {
		return fmt.Errorf("%s exceeds the %d-byte limit", name, maxLifecycleControlTextBytes)
	}
	return nil
}

func lifecycleControlBodyStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) || strings.Contains(err.Error(), "transport bound") {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}
