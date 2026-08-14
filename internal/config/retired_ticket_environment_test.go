package config

import (
	"os"
	"strings"
	"testing"
)

func TestRetiredTicketInferenceEnvironmentFailsLoudly(t *testing.T) {
	t.Setenv("OMNI_TICKET_CONTEXT_DEADLINE", "10s")
	if err := validateTypedEnvironment(); err == nil || !strings.Contains(err.Error(), "was removed") {
		t.Fatalf("retired ticket inference setting error=%v", err)
	}
}

func TestManagedEnvironmentTemplatesDoNotAdvertiseRetiredTicketInference(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"../../default.env", "../../.env.example"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "OMNI_TICKET_CONTEXT_DEADLINE") {
			t.Errorf("%s retains retired ticket inference configuration", path)
		}
	}
}
