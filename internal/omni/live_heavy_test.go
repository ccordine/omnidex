package omni

import (
	"os"
	"strings"
	"testing"
)

func skipUnlessHeavyLiveEnabled(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("OMNI_RUN_HEAVY_LIVE")) == "" {
		t.Skip("set OMNI_RUN_HEAVY_LIVE=1 to run heavy live Ollama build tests")
	}
}

func skipUnlessLiveOllamaEnabled(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("OMNI_RUN_LIVE_OLLAMA")) == "" {
		t.Skip("set OMNI_RUN_LIVE_OLLAMA=1 to run live Ollama integration tests")
	}
}
