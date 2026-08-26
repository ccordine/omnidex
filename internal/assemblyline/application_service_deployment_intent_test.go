package assemblyline

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestApplicationServiceDeploymentIntentHasOneOpaqueResponsibility(t *testing.T) {
	input := ApplicationServiceDeploymentIntentInput{
		UserRequest: "Build a public status display and keep it running in this environment.",
	}
	job, err := NewApplicationServiceDeploymentIntentJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkApplicationServiceDeploymentIntent {
		t.Fatalf("kind=%q", job.Kind)
	}
	var payload map[string]any
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || payload["user_request"] != input.UserRequest {
		t.Fatalf("model input exceeds the one immutable request: %#v", payload)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(prompt, input.UserRequest) != 1 {
		t.Fatalf("immutable request count=%d want 1", strings.Count(prompt, input.UserRequest))
	}
	for _, forbidden := range []string{
		"command", "credential", "health", "workspace", "filename", "tool", "source code", "completion status",
	} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("deployment-intent prompt exposed %q authority: %s", forbidden, prompt)
		}
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) != 2 {
		t.Fatalf("response properties=%#v want schema and one semantic leaf", schema["properties"])
	}
	required, ok := schema["required"].([]string)
	if !ok || len(required) != 2 || required[0] != "schema" || required[1] != "candidate_id" {
		t.Fatalf("required=%#v", schema["required"])
	}
	candidateProperty, ok := properties["candidate_id"].(map[string]any)
	if !ok {
		t.Fatalf("candidate property=%#v", properties["candidate_id"])
	}
	values, ok := candidateProperty["enum"].([]string)
	if !ok || len(values) != 3 {
		t.Fatalf("candidate enum=%#v", candidateProperty["enum"])
	}
	opaque := regexp.MustCompile(`^DEPLOYMENT_CANDIDATE_[1-3]$`)
	for _, value := range values {
		if !opaque.MatchString(value) {
			t.Fatalf("candidate ID %q exposes semantic control", value)
		}
	}
}

func TestApplicationServiceDeploymentIntentDecodesUnrelatedFixtures(t *testing.T) {
	fixtures := []struct {
		name        string
		request     string
		candidate   ApplicationServiceDeploymentCandidateID
		disposition ApplicationServiceDeploymentDisposition
	}{
		{
			name:        "transient conversion utility",
			request:     "Create a unit conversion utility.",
			candidate:   ApplicationServiceDeploymentNoPersistenceCandidate,
			disposition: ApplicationServiceDeploymentVerifyOnly,
		},
		{
			name:        "persistent status display",
			request:     "Build a public status display and keep it running in this environment.",
			candidate:   ApplicationServiceDeploymentCurrentHostCandidate,
			disposition: ApplicationServiceDeploymentPersistCurrentHost,
		},
		{
			name:        "external data viewer",
			request:     "Publish a data viewer to a separate hosted destination and keep it available.",
			candidate:   ApplicationServiceDeploymentOtherTargetCandidate,
			disposition: ApplicationServiceDeploymentTargetUnresolved,
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			input := ApplicationServiceDeploymentIntentInput{UserRequest: fixture.request}
			raw := `{"schema":"` + ApplicationServiceDeploymentIntentSchemaV1 +
				`","candidate_id":"` + string(fixture.candidate) + `"}`
			result, err := DecodeApplicationServiceDeploymentIntentResult(input, raw)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ResolveApplicationServiceDeploymentDisposition(result)
			if err != nil {
				t.Fatal(err)
			}
			if got != fixture.disposition {
				t.Fatalf("disposition=%q want=%q", got, fixture.disposition)
			}
		})
	}
}

func TestApplicationServiceDeploymentIntentFailsClosedOnMalformedResults(t *testing.T) {
	input := ApplicationServiceDeploymentIntentInput{UserRequest: "Create a unit conversion utility."}
	valid := `{"schema":"` + ApplicationServiceDeploymentIntentSchemaV1 +
		`","candidate_id":"` + string(ApplicationServiceDeploymentNoPersistenceCandidate) + `"}`
	for name, raw := range map[string]string{
		"unknown candidate": strings.Replace(valid, "DEPLOYMENT_CANDIDATE_1", "DEPLOYMENT_CANDIDATE_9", 1),
		"wrong schema":      strings.Replace(valid, ApplicationServiceDeploymentIntentSchemaV1, "omnidex.invalid.v1", 1),
		"extra field":       strings.TrimSuffix(valid, "}") + `,"command":"start"}`,
		"duplicate field":   strings.TrimSuffix(valid, "}") + `,"candidate_id":"DEPLOYMENT_CANDIDATE_2"}`,
		"trailing value":    valid + `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeApplicationServiceDeploymentIntentResult(input, raw); err == nil {
				t.Fatal("accepted malformed deployment-intent result")
			}
		})
	}
}

func TestApplicationServiceDeploymentIntentRejectsPathBearingAuthority(t *testing.T) {
	input := ApplicationServiceDeploymentIntentInput{
		UserRequest: "Keep the software from /workspace/generated running.",
	}
	if _, err := NewApplicationServiceDeploymentIntentJob(input); err == nil ||
		!strings.Contains(err.Error(), "filesystem identity") {
		t.Fatalf("path-bearing deployment intent error=%v", err)
	}
}

func TestApplicationServiceDeploymentIntentProductionSourceHasNoWorkloadRecipe(t *testing.T) {
	files := []string{
		"application_service_deployment_intent.go",
		"application_service_deployment_intent_station.go",
	}
	forbidden := []string{
		strings.Join([]string{"no", "te"}, ""),
		strings.Join([]string{"sequen", "cer"}, ""),
		strings.Join([]string{"appoint", "ment"}, ""),
		strings.Join([]string{"in", "voice"}, ""),
	}
	for _, name := range files {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(raw))
		for _, noun := range forbidden {
			if strings.Contains(lower, noun) {
				t.Fatalf("production deployment-intent source %s contains workload noun %q", name, noun)
			}
		}
	}
}

func TestApplicationIntentPromptSeparatesPostVerificationDisposition(t *testing.T) {
	request := "Build a public status display and keep it running in this environment."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty, nil)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildApplicationIntentPrompt(ApplicationIntentInput{
		UserRequest: request,
		Context:     context,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "omit inferred implementation detail, unstated product obligations, and post-verification runtime disposition") {
		t.Fatalf("application intent prompt does not isolate runtime disposition: %s", prompt)
	}
}
