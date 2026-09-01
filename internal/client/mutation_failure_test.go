package client

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestDefinitiveMutationRejectionClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		definitive bool
	}{
		{name: "request timeout", err: &HTTPError{StatusCode: http.StatusRequestTimeout}},
		{name: "too early", err: &HTTPError{StatusCode: http.StatusTooEarly}},
		{name: "too many requests", err: &HTTPError{StatusCode: http.StatusTooManyRequests}},
		{name: "unexpected client response", err: &HTTPError{StatusCode: http.StatusTeapot}},
		{name: "intermediary failure", err: &HTTPError{StatusCode: http.StatusBadGateway}},
		{name: "dropped transport", err: errors.New("connection dropped")},
		{name: "bad request", err: &HTTPError{StatusCode: http.StatusBadRequest}, definitive: true},
		{name: "not found", err: &HTTPError{StatusCode: http.StatusNotFound}, definitive: true},
		{name: "conflict", err: &HTTPError{StatusCode: http.StatusConflict}, definitive: true},
		{name: "wrapped rejection", err: fmt.Errorf("submit mutation: %w", &HTTPError{StatusCode: http.StatusRequestEntityTooLarge}), definitive: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if actual := IsDefinitiveMutationRejection(test.err); actual != test.definitive {
				t.Fatalf("classification = %t, want %t for %v", actual, test.definitive, test.err)
			}
		})
	}
}
