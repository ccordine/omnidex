package queue

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const jobHistoryCursorVersion = "jh1"

func validateJobHistoryRequest(jobID int64, request JobHistoryRequest) (int64, error) {
	if jobID <= 0 {
		return 0, fmt.Errorf("%w: positive job ID is required", ErrInvalidJobHistoryRequest)
	}
	if !registeredJobHistoryStream(request.Stream) {
		return 0, fmt.Errorf("%w: stream %q is not registered", ErrInvalidJobHistoryRequest, request.Stream)
	}
	if request.Limit < 1 || request.Limit > MaxJobHistoryPageSize {
		return 0, fmt.Errorf(
			"%w: limit must be between 1 and %d", ErrInvalidJobHistoryRequest, MaxJobHistoryPageSize,
		)
	}
	return decodeJobHistoryCursor(request.Cursor, jobID, request.Stream)
}

func registeredJobHistoryStream(stream JobHistoryStream) bool {
	switch stream {
	case JobHistoryGenerations, JobHistorySteps, JobHistoryArtifacts,
		JobHistoryEvidence, JobHistoryClaims, JobHistoryLLMCalls:
		return true
	default:
		return false
	}
}

func encodeJobHistoryCursor(jobID int64, stream JobHistoryStream, position int64) (string, error) {
	if jobID <= 0 || position <= 0 || !registeredJobHistoryStream(stream) {
		return "", fmt.Errorf("%w: cursor authority is invalid", ErrInvalidJobHistoryRequest)
	}
	payload := fmt.Sprintf("%s:%d:%s:%d", jobHistoryCursorVersion, jobID, stream, position)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)), nil
}

func decodeJobHistoryCursor(raw string, jobID int64, stream JobHistoryStream) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	if raw != strings.TrimSpace(raw) || len(raw) > 512 {
		return 0, fmt.Errorf("%w: cursor is malformed", ErrInvalidJobHistoryRequest)
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: cursor is malformed", ErrInvalidJobHistoryRequest)
	}
	parts := strings.Split(string(decoded), ":")
	if len(parts) != 4 || parts[0] != jobHistoryCursorVersion ||
		parts[1] != strconv.FormatInt(jobID, 10) || parts[2] != string(stream) {
		return 0, fmt.Errorf("%w: cursor does not belong to this job and stream", ErrInvalidJobHistoryRequest)
	}
	position, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || position <= 0 {
		return 0, fmt.Errorf("%w: cursor position is invalid", ErrInvalidJobHistoryRequest)
	}
	return position, nil
}

func finishJobHistoryPage[T any](
	jobID int64,
	stream JobHistoryStream,
	items []T,
	limit int,
	position func(T) int64,
) ([]T, string, error) {
	if len(items) <= limit {
		return items, "", nil
	}
	items = items[:limit]
	cursor, err := encodeJobHistoryCursor(jobID, stream, position(items[len(items)-1]))
	if err != nil {
		return nil, "", err
	}
	return items, cursor, nil
}
