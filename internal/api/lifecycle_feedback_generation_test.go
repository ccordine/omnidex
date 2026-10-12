package api

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLifecycleFeedbackGenerationHasExactPositiveIntegerShape(t *testing.T) {
	for _, test := range []struct {
		name, value string
		valid       bool
	}{
		{"positive", "2", true}, {"zero", "0", false}, {"negative", "-1", false},
		{"fraction", "1.5", false}, {"string", `"2"`, false}, {"overflow", "9223372036854775808", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"operation_id":"lifecycle_operation_generation_fixture","feedback":"Use the approved choice.","expected_generation":%s}`, test.value)
			request, err := decodeLifecycleFeedbackRequest(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/jobs/1/feedback", strings.NewReader(body)))
			if (err == nil) != test.valid {
				t.Fatalf("generation value %s: request=%#v error=%v", test.value, request, err)
			}
			if test.valid && (request.ExpectedGeneration == nil || *request.ExpectedGeneration != 2) {
				t.Fatalf("observed generation was not preserved: %#v", request)
			}
		})
	}
	body := `{"operation_id":"lifecycle_operation_normal_generation_fixture","feedback":"Normal feedback."}`
	request, err := decodeLifecycleFeedbackRequest(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/jobs/1/feedback", strings.NewReader(body)))
	if err != nil || request.ExpectedGeneration != nil {
		t.Fatalf("normal absent generation changed: %#v %v", request, err)
	}
}

func TestLifecycleFeedbackWaitingAttemptHasExactPositiveIntegerShape(t *testing.T) {
	for _, test := range []struct {
		name, value string
		valid       bool
	}{{"positive", "2", true}, {"zero", "0", false}, {"negative", "-1", false}, {"fraction", "1.5", false}, {"string", `"2"`, false}, {"overflow", "9223372036854775808", false}} {
		t.Run(test.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"operation_id":"lifecycle_operation_attempt_fixture","feedback":"Use the approved choice.","expected_waiting_attempt":%s}`, test.value)
			request, err := decodeLifecycleFeedbackRequest(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/jobs/1/feedback", strings.NewReader(body)))
			if (err == nil) != test.valid {
				t.Fatalf("waiting attempt %s: request=%#v error=%v", test.value, request, err)
			}
			if test.valid && (request.ExpectedWaitingAttempt == nil || *request.ExpectedWaitingAttempt != 2) {
				t.Fatalf("waiting attempt lost: %#v", request)
			}
		})
	}
}
