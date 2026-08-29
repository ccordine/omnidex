package queue

import (
	"errors"
	"strings"
	"testing"
)

func TestJobHistoryRequestRejectsInvalidBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		jobID   int64
		request JobHistoryRequest
		want    string
	}{
		{name: "job", request: JobHistoryRequest{Stream: JobHistoryGenerations, Limit: 1}, want: "positive job"},
		{name: "stream", jobID: 1, request: JobHistoryRequest{Stream: "everything", Limit: 1}, want: "stream"},
		{name: "retired claims stream", jobID: 1, request: JobHistoryRequest{Stream: "claims", Limit: 1}, want: "stream"},
		{name: "zero limit", jobID: 1, request: JobHistoryRequest{Stream: JobHistorySteps}, want: "limit"},
		{name: "large limit", jobID: 1, request: JobHistoryRequest{Stream: JobHistorySteps, Limit: MaxJobHistoryPageSize + 1}, want: "limit"},
		{name: "cursor whitespace", jobID: 1, request: JobHistoryRequest{Stream: JobHistorySteps, Limit: 1, Cursor: " bad "}, want: "cursor"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (&Repository{}).ReadJobHistoryPage(t.Context(), test.jobID, test.request)
			if !errors.Is(err, ErrInvalidJobHistoryRequest) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want invalid history request containing %q", err, test.want)
			}
		})
	}
}

func TestJobHistoryCursorIsOpaqueAndBoundToAuthority(t *testing.T) {
	t.Parallel()

	cursor, err := encodeJobHistoryCursor(42, JobHistoryArtifacts, 91)
	if err != nil {
		t.Fatal(err)
	}
	if cursor == "" || strings.Contains(cursor, "42") || strings.Contains(cursor, "artifacts") {
		t.Fatalf("cursor is not opaque: %q", cursor)
	}
	position, err := decodeJobHistoryCursor(cursor, 42, JobHistoryArtifacts)
	if err != nil || position != 91 {
		t.Fatalf("decoded position=%d error=%v", position, err)
	}
	for _, mismatch := range []struct {
		jobID  int64
		stream JobHistoryStream
	}{
		{jobID: 43, stream: JobHistoryArtifacts},
		{jobID: 42, stream: JobHistoryEvidence},
	} {
		if _, err := decodeJobHistoryCursor(cursor, mismatch.jobID, mismatch.stream); !errors.Is(err, ErrInvalidJobHistoryRequest) {
			t.Fatalf("cross-authority cursor error=%v", err)
		}
	}
	if _, err := decodeJobHistoryCursor(cursor+"!", 42, JobHistoryArtifacts); !errors.Is(err, ErrInvalidJobHistoryRequest) {
		t.Fatalf("malformed cursor error=%v", err)
	}
}
