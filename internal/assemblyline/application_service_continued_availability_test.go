package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplicationServiceContinuedAvailabilityHasOneOpaqueResponsibility(t *testing.T) {
	input := ApplicationServiceContinuedAvailabilityInput{
		UserRequest: "Build a public status display and keep it running.",
	}
	job, err := NewApplicationServiceContinuedAvailabilityJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkApplicationServiceContinuedAvailability {
		t.Fatalf("kind=%q", job.Kind)
	}
	var payload map[string]any
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || payload["user_request"] != input.UserRequest {
		t.Fatalf("model input exceeds the immutable request: %#v", payload)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(prompt, input.UserRequest) != 1 {
		t.Fatalf("immutable request count=%d want 1", strings.Count(prompt, input.UserRequest))
	}
	if strings.Contains(prompt, "DESTINATION_CANDIDATE_") ||
		strings.Contains(prompt, ApplicationServicePersistenceDestinationSchemaV1) ||
		strings.Contains(prompt, "identifies the environment where the software is built") {
		t.Fatalf("continued-availability prompt contains destination responsibility: %s", prompt)
	}
}

func TestApplicationServiceContinuedAvailabilityDecodesBothCandidates(t *testing.T) {
	input := ApplicationServiceContinuedAvailabilityInput{UserRequest: "Create a conversion utility."}
	for _, candidate := range []ApplicationServiceContinuedAvailabilityCandidateID{
		ApplicationServiceAvailabilityNotRequiredCandidate,
		ApplicationServiceAvailabilityRequiredCandidate,
	} {
		raw := string(candidate)
		if _, err := DecodeApplicationServiceContinuedAvailabilityResult(input, raw); err != nil {
			t.Fatalf("candidate=%q error=%v", candidate, err)
		}
	}
}

func TestApplicationServiceContinuedAvailabilityFailsClosed(t *testing.T) {
	input := ApplicationServiceContinuedAvailabilityInput{UserRequest: "Create a conversion utility."}
	for name, raw := range map[string]string{
		"unknown candidate": "AVAILABILITY_CANDIDATE_9",
		"JSON wrapper":      `{"candidate_id":"AVAILABILITY_CANDIDATE_1"}`,
		"quoted":            `"AVAILABILITY_CANDIDATE_1"`,
		"label":             "candidate_id: AVAILABILITY_CANDIDATE_1",
		"trailing value":    "AVAILABILITY_CANDIDATE_1 AVAILABILITY_CANDIDATE_2",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeApplicationServiceContinuedAvailabilityResult(input, raw); err == nil {
				t.Fatal("accepted malformed continued-availability result")
			}
		})
	}
}

func TestApplicationServiceContinuedAvailabilityRejectsPathBearingAuthority(t *testing.T) {
	input := ApplicationServiceContinuedAvailabilityInput{
		UserRequest: "Keep the software from /workspace/generated running.",
	}
	if _, err := NewApplicationServiceContinuedAvailabilityJob(input); err == nil ||
		!strings.Contains(err.Error(), "filesystem identity") {
		t.Fatalf("path-bearing continued availability error=%v", err)
	}
}
