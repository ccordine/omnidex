package queue

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestReplayReplacementGenerationIgnoresMalformedObservationTimestamp(t *testing.T) {
	original := llm.PreparedGeneration{
		Schema:             llm.PreparedGenerationSchemaV1,
		Protocol:           llm.ExactPreparedProtocolRawTextV2,
		Content:            "bounded partial response",
		ProviderHTTPStatus: 200,
		ProviderObservation: llm.ProviderIdentityObservation{
			ObservedAt: time.Date(2026, 8, 29, 3, 0, 0, 0, time.UTC),
		},
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	malformed := strings.Replace(
		string(raw),
		`"observed_at":"2026-08-29T03:00:00Z"`,
		`"observed_at":"not-a-provider-observation-time"`,
		1,
	)
	if malformed == string(raw) {
		t.Fatal("replacement replay fixture did not mutate provider observation time")
	}

	decoded, err := decodeStationCallReplayReplacementGeneration([]byte(malformed))
	if err != nil {
		t.Fatalf("purpose-specific replacement receipt decode: %v", err)
	}
	if decoded.Content != original.Content ||
		decoded.ProviderHTTPStatus != original.ProviderHTTPStatus ||
		decoded.ProviderObservation != (llm.ProviderIdentityObservation{}) {
		t.Fatalf("replacement receipt projection=%+v", decoded)
	}
}
