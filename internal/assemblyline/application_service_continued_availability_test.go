package assemblyline

import (
	"encoding/json"
	"regexp"
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
	prompt, schema, err := RenderPortableJob(job)
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
	assertBinaryOpaqueServiceSemanticSchema(
		t, schema, ApplicationServiceContinuedAvailabilitySchemaV1, `^AVAILABILITY_CANDIDATE_[12]$`,
	)
}

func TestApplicationServiceContinuedAvailabilityDecodesBothCandidates(t *testing.T) {
	input := ApplicationServiceContinuedAvailabilityInput{UserRequest: "Create a conversion utility."}
	for _, candidate := range []ApplicationServiceContinuedAvailabilityCandidateID{
		ApplicationServiceAvailabilityNotRequiredCandidate,
		ApplicationServiceAvailabilityRequiredCandidate,
	} {
		raw := `{"schema":"` + ApplicationServiceContinuedAvailabilitySchemaV1 +
			`","candidate_id":"` + string(candidate) + `"}`
		if _, err := DecodeApplicationServiceContinuedAvailabilityResult(input, raw); err != nil {
			t.Fatalf("candidate=%q error=%v", candidate, err)
		}
	}
}

func TestApplicationServiceContinuedAvailabilityFailsClosed(t *testing.T) {
	input := ApplicationServiceContinuedAvailabilityInput{UserRequest: "Create a conversion utility."}
	valid := `{"schema":"` + ApplicationServiceContinuedAvailabilitySchemaV1 +
		`","candidate_id":"` + string(ApplicationServiceAvailabilityNotRequiredCandidate) + `"}`
	for name, raw := range map[string]string{
		"unknown candidate": strings.Replace(valid, "AVAILABILITY_CANDIDATE_1", "AVAILABILITY_CANDIDATE_9", 1),
		"wrong schema":      strings.Replace(valid, ApplicationServiceContinuedAvailabilitySchemaV1, "omnidex.invalid.v1", 1),
		"extra field":       strings.TrimSuffix(valid, "}") + `,"destination":"somewhere"}`,
		"duplicate field":   strings.TrimSuffix(valid, "}") + `,"candidate_id":"AVAILABILITY_CANDIDATE_2"}`,
		"trailing value":    valid + `{}`,
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

func assertBinaryOpaqueServiceSemanticSchema(
	t *testing.T, schema map[string]any, wantSchema, candidatePattern string,
) {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) != 2 {
		t.Fatalf("response properties=%#v want schema and one semantic leaf", schema["properties"])
	}
	required, ok := schema["required"].([]string)
	if !ok || len(required) != 2 || required[0] != "schema" || required[1] != "candidate_id" {
		t.Fatalf("required=%#v", schema["required"])
	}
	schemaProperty, ok := properties["schema"].(map[string]any)
	if !ok || schemaProperty["const"] != wantSchema {
		t.Fatalf("schema const=%#v want=%q", schemaProperty["const"], wantSchema)
	}
	candidateProperty, ok := properties["candidate_id"].(map[string]any)
	if !ok {
		t.Fatalf("candidate property=%#v", properties["candidate_id"])
	}
	values, ok := candidateProperty["enum"].([]string)
	if !ok || len(values) != 2 {
		t.Fatalf("candidate enum=%#v", candidateProperty["enum"])
	}
	opaque := regexp.MustCompile(candidatePattern)
	for _, value := range values {
		if !opaque.MatchString(value) {
			t.Fatalf("candidate ID %q exposes semantic control", value)
		}
	}
}
