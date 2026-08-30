package worker

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/ollama"
)

func newLiveApplicationSemanticBoundaryTransport(
	t *testing.T,
	scope string,
) (context.Context, string, *liveCodingQualificationTransport) {
	t.Helper()
	modelName := strings.TrimSpace(os.Getenv(liveCodingQualificationModelEnv))
	if modelName == "" {
		t.Skip(liveCodingQualificationModelEnv + " is not set")
	}
	baseURL := requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_URL")
	contextTokens, err := strconv.Atoi(
		requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_CONTEXT"),
	)
	if err != nil || contextTokens <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Minute)
	t.Cleanup(cancel)
	client := ollama.New(baseURL, modelName, "", 5*time.Minute, contextTokens)
	transport, err := newLiveCodingQualificationTransport(
		ctx,
		client,
		modelName,
		contextTokens,
		scope,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, modelName, transport
}
