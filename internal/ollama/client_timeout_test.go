package ollama

import (
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestClientCannotDisableOrExceedModelRequestTimeout(t *testing.T) {
	for _, test := range []struct {
		name      string
		requested time.Duration
		want      time.Duration
	}{
		{name: "shorter", requested: 9 * time.Minute, want: 9 * time.Minute},
		{name: "maximum", requested: llm.MaximumModelRequestDuration, want: llm.MaximumModelRequestDuration},
		{name: "over maximum", requested: 45 * time.Minute, want: llm.MaximumModelRequestDuration},
		{name: "zero", requested: 0, want: llm.MaximumModelRequestDuration},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newClient("http://localhost:11434", "", "", test.requested)
			if got := client.httpClient.Timeout; got != test.want {
				t.Fatalf("http timeout=%s, want %s", got, test.want)
			}
		})
	}
}
