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
)

const (
	maxChannelCreateBodyBytes  int64 = 32 * 1024
	maxChannelMessageBodyBytes int64 = 16 * 1024
)

func decodeExactChannelJSON(
	w http.ResponseWriter,
	r *http.Request,
	name string,
	maxBytes int64,
	destination any,
) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return channelBodyError{Status: http.StatusRequestEntityTooLarge, Err: fmt.Errorf(
				"%s exceeds the %d-byte transport bound", name, maxBytes,
			)}
		}
		return channelBodyError{Status: http.StatusBadRequest, Err: fmt.Errorf("read %s: %w", name, err)}
	}
	if !utf8.Valid(raw) {
		return channelBodyError{Status: http.StatusBadRequest, Err: fmt.Errorf("%s must be valid UTF-8", name)}
	}
	if err := exactjson.ValidateObject(raw, destination, name); err != nil {
		return channelBodyError{Status: http.StatusBadRequest, Err: err}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return channelBodyError{Status: http.StatusRequestEntityTooLarge, Err: fmt.Errorf(
				"%s exceeds the %d-byte transport bound", name, maxBytes,
			)}
		}
		return channelBodyError{Status: http.StatusBadRequest, Err: fmt.Errorf("invalid %s JSON: %w", name, err)}
	}
	if err := requireJSONEOF(decoder, name); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return channelBodyError{Status: http.StatusRequestEntityTooLarge, Err: fmt.Errorf(
				"%s exceeds the %d-byte transport bound", name, maxBytes,
			)}
		}
		return channelBodyError{Status: http.StatusBadRequest, Err: err}
	}
	return nil
}

type channelBodyError struct {
	Status int
	Err    error
}

func (err channelBodyError) Error() string { return err.Err.Error() }

func writeChannelBodyError(w http.ResponseWriter, err error) {
	var bodyErr channelBodyError
	if !errors.As(err, &bodyErr) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeError(w, bodyErr.Status, bodyErr.Error())
}
