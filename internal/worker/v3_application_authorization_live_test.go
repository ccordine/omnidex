package worker

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
)

func TestLiveRequirementAuthorizationPreservesRequestedBehavior(t *testing.T) {
	modelName := strings.TrimSpace(os.Getenv(liveRequirementsModelEnv))
	if modelName == "" {
		t.Skip(liveRequirementsModelEnv + " is not set")
	}
	baseURL := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_OLLAMA_URL"))
	if baseURL == "" {
		t.Fatal("OMNIDEX_TEST_OLLAMA_URL is required")
	}
	contextTokens, err := strconv.Atoi(os.Getenv("OMNIDEX_TEST_OLLAMA_CONTEXT"))
	if err != nil || contextTokens <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	client := ollama.New(baseURL, modelName, "", llm.MaximumModelRequestDuration)
	for _, fixture := range []struct {
		name, request, required, additional string
	}{
		{
			name:       "ordering",
			request:    "Build a JavaScript tool that alphabetizes the supplied names.",
			required:   "The software alphabetizes the supplied names.",
			additional: "The software alphabetizes the supplied names and uploads them to a remote server.",
		},
		{
			name:       "status",
			request:    "Build a browser app that lets the user mark an invoice as paid.",
			required:   "The software lets the user mark an invoice as paid.",
			additional: "The software lets the user mark an invoice as paid and emails a receipt.",
		},
		{
			name:       "part of a larger request",
			request:    "Build software that schedules appointments at supplied times and cancels selected appointments.",
			required:   "The software schedules appointments at supplied times.",
			additional: "The software schedules appointments at supplied times and emails reminders.",
		},
	} {
		for _, candidate := range []struct {
			name, statement, want string
		}{
			{"requested", fixture.required, assemblyline.ApplicationRequirementCandidateEntailed},
			{"added", fixture.additional, assemblyline.ApplicationRequirementCandidateNotEntailed},
		} {
			t.Run(fixture.name+"/"+candidate.name, func(t *testing.T) {
				context, err := assemblyline.BootstrapApplicationContext(fixture.request)
				if err != nil {
					t.Fatal(err)
				}
				input := assemblyline.ApplicationRequirementCandidateAuthorizationInput{
					UserRequest: fixture.request, Context: context, Candidate: candidate.statement,
				}
				job, err := assemblyline.NewApplicationRequirementCandidateAuthorizationJob(input)
				if err != nil {
					t.Fatal(err)
				}
				response, err := executeLiveRequirementsSemanticJob(
					t.Context(), client, contextTokens, modelName, job, t,
				)
				if err != nil {
					t.Fatal(err)
				}
				result, err := assemblyline.DecodeApplicationRequirementCandidateAuthorizationResult(input, response.Candidate)
				if err != nil {
					t.Fatal(err)
				}
				if result.Relation != candidate.want {
					t.Fatalf("authorization=%s want %s", result.Relation, candidate.want)
				}
			})
		}
	}
}
