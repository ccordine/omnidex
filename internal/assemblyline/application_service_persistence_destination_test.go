package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplicationServicePersistenceDestinationHasOneOpaqueResponsibility(t *testing.T) {
	input := ApplicationServicePersistenceDestinationInput{
		UserRequest:           "Build a public status display and keep it running in this environment.",
		ContinuedAvailability: requiredContinuedAvailabilityResult(),
	}
	job, err := NewApplicationServicePersistenceDestinationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkApplicationServicePersistenceDestination {
		t.Fatalf("kind=%q", job.Kind)
	}
	var payload map[string]any
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 2 || payload["user_request"] != input.UserRequest ||
		payload["continued_availability"] == nil {
		t.Fatalf("portable input does not bind the request and affirmative authority: %#v", payload)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(prompt, input.UserRequest) != 1 {
		t.Fatalf("immutable request count=%d want 1", strings.Count(prompt, input.UserRequest))
	}
	if strings.Contains(prompt, "AVAILABILITY_CANDIDATE_") ||
		strings.Contains(prompt, ApplicationServiceContinuedAvailabilitySchemaV1) ||
		strings.Contains(prompt, "whether the immutable request explicitly requires") {
		t.Fatalf("persistence-destination prompt contains availability responsibility: %s", prompt)
	}
	assertBinaryOpaqueServiceSemanticSchema(
		t, schema, ApplicationServicePersistenceDestinationSchemaV1, `^DESTINATION_CANDIDATE_[12]$`,
	)
}

func TestApplicationServicePersistenceDestinationDecodesBothCandidates(t *testing.T) {
	input := ApplicationServicePersistenceDestinationInput{
		UserRequest:           "Keep the completed service running in this environment.",
		ContinuedAvailability: requiredContinuedAvailabilityResult(),
	}
	for _, candidate := range []ApplicationServicePersistenceDestinationCandidateID{
		ApplicationServiceBuildEnvironmentDestinationCandidate,
		ApplicationServiceBuildEnvironmentNotEstablishedCandidate,
	} {
		raw := `{"schema":"` + ApplicationServicePersistenceDestinationSchemaV1 +
			`","candidate_id":"` + string(candidate) + `"}`
		if _, err := DecodeApplicationServicePersistenceDestinationResult(input, raw); err != nil {
			t.Fatalf("candidate=%q error=%v", candidate, err)
		}
	}
}

func TestApplicationServicePersistenceDestinationFailsClosed(t *testing.T) {
	input := ApplicationServicePersistenceDestinationInput{
		UserRequest:           "Keep the completed service running in this environment.",
		ContinuedAvailability: requiredContinuedAvailabilityResult(),
	}
	valid := `{"schema":"` + ApplicationServicePersistenceDestinationSchemaV1 +
		`","candidate_id":"` + string(ApplicationServiceBuildEnvironmentDestinationCandidate) + `"}`
	for name, raw := range map[string]string{
		"unknown candidate": strings.Replace(valid, "DESTINATION_CANDIDATE_1", "DESTINATION_CANDIDATE_9", 1),
		"wrong schema":      strings.Replace(valid, ApplicationServicePersistenceDestinationSchemaV1, "omnidex.invalid.v1", 1),
		"extra field":       strings.TrimSuffix(valid, "}") + `,"availability":"required"}`,
		"duplicate field":   strings.TrimSuffix(valid, "}") + `,"candidate_id":"DESTINATION_CANDIDATE_2"}`,
		"trailing value":    valid + `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeApplicationServicePersistenceDestinationResult(input, raw); err == nil {
				t.Fatal("accepted malformed persistence-destination result")
			}
		})
	}
}

func TestApplicationServicePersistenceDestinationRejectsPathBearingAuthority(t *testing.T) {
	input := ApplicationServicePersistenceDestinationInput{
		UserRequest:           "Keep the software from /workspace/generated running.",
		ContinuedAvailability: requiredContinuedAvailabilityResult(),
	}
	if _, err := NewApplicationServicePersistenceDestinationJob(input); err == nil ||
		!strings.Contains(err.Error(), "filesystem identity") {
		t.Fatalf("path-bearing persistence destination error=%v", err)
	}
}

func TestApplicationServicePersistenceDestinationRequiresAffirmativeAvailability(t *testing.T) {
	input := ApplicationServicePersistenceDestinationInput{
		UserRequest: "Build the service.",
		ContinuedAvailability: ApplicationServiceContinuedAvailabilityResult{
			Schema:      ApplicationServiceContinuedAvailabilitySchemaV1,
			CandidateID: ApplicationServiceAvailabilityNotRequiredCandidate,
		},
	}
	if _, err := NewApplicationServicePersistenceDestinationJob(input); err == nil ||
		!strings.Contains(err.Error(), "requires explicit continued-availability authority") {
		t.Fatalf("non-affirmative destination input error=%v", err)
	}
}

func requiredContinuedAvailabilityResult() ApplicationServiceContinuedAvailabilityResult {
	return ApplicationServiceContinuedAvailabilityResult{
		Schema:      ApplicationServiceContinuedAvailabilitySchemaV1,
		CandidateID: ApplicationServiceAvailabilityRequiredCandidate,
	}
}
