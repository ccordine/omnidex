package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProviderContentEncodingClassifiesOnlyDecodingSupport(t *testing.T) {
	for _, fixture := range []struct {
		values       []string
		uncompressed bool
		want         ProviderContentEncoding
	}{
		{nil, false, ProviderEncodingIdentity},
		{[]string{"identity"}, false, ProviderEncodingIdentity},
		{[]string{"IDENTITY"}, false, ProviderEncodingIdentity},
		{[]string{"gzip"}, false, ProviderEncodingUnsupported},
		{[]string{"identity", "gzip"}, false, ProviderEncodingUnsupported},
		{nil, true, ProviderEncodingUnsupported},
	} {
		got := ClassifyProviderContentEncoding(fixture.values, fixture.uncompressed)
		if got != fixture.want || got.Validate() != nil {
			t.Fatalf("encoding %v uncompressed=%t: %q; want %q", fixture.values, fixture.uncompressed, got, fixture.want)
		}
	}
	if ProviderContentEncoding("").Validate() == nil {
		t.Fatal("missing encoding fact was accepted")
	}
}

func TestPreparedGenerationUsesResponseContentWithoutHashReceipts(t *testing.T) {
	prepared := exactPreparedRequestFixture()
	generation := exactPreparedGenerationFixture(t, prepared, "stop", 40, 12)
	if err := ValidateExactPreparedGenerationForRequest(prepared, generation); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(generation)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sha256") || strings.Contains(string(raw), "captured_base64") {
		t.Fatalf("provider result contains redundant receipts: %s", raw)
	}
	generation.Content = "unreturned value"
	if err := ValidateExactPreparedGenerationForRequest(prepared, generation); err == nil {
		t.Fatal("content different from the provider response was accepted")
	}
}
