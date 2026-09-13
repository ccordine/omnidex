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

func TestLiveApplicationSurfacePreservesExplicitInterface(t *testing.T) {
	modelName := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_CODING_SURFACE_MODEL"))
	if modelName == "" {
		t.Skip("OMNIDEX_TEST_CODING_SURFACE_MODEL is not set")
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
		name, request string
		want          assemblyline.ApplicationSurface
	}{
		{"directory measurement", "Create a small Go command-line program that reports the size of the directory supplied as its argument.", assemblyline.ApplicationSurfaceCommandLine},
		{"document validation", "Create a small JavaScript command-line program that validates the JSON document supplied as its argument.", assemblyline.ApplicationSurfaceCommandLine},
		{"date selection", "Build a browser app that lets a user select a delivery date.", assemblyline.ApplicationSurfaceBrowser},
		{"unsupported technology", "Build a PHP command-line program that formats supplied dates.", assemblyline.ApplicationSurfaceCommandLine},
		{"unspecified interface", "The software records whether a parcel has arrived.", assemblyline.ApplicationSurfaceUnspecified},
		{"unsupported interface", "Build a native iOS app that tracks rest periods.", assemblyline.ApplicationSurfaceUnsupported},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			input := assemblyline.ApplicationClassificationInput{UserRequest: fixture.request}
			job, err := assemblyline.NewApplicationClassificationJob(input)
			if err != nil {
				t.Fatal(err)
			}
			response, err := executeLiveRequirementsSemanticJob(t.Context(), client, contextTokens, modelName, job, t)
			if err != nil {
				t.Fatal(err)
			}
			result, err := assemblyline.DecodeApplicationClassification(input, response.Candidate)
			if err != nil {
				t.Fatal(err)
			}
			if result.Surface != fixture.want {
				t.Fatalf("surface=%s want %s", result.Surface, fixture.want)
			}
		})
	}
}
