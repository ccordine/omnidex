package ollama

import (
	"os"
	"strings"
	"testing"
)

func TestExactRawTextTransportHasNoChatOrFormatSidePath(t *testing.T) {
	raw, err := os.ReadFile("prepared_raw.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		`c.baseURL+"/api/generate"`,
		"ExactPreparedRequestBytes(prepared)",
		"DecodeExactPreparedResponseForProtocol(",
		"prepared.Protocol",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("exact raw transport lacks %q", required)
		}
	}
	for _, forbidden := range []string{`/api/chat`, `ResponseSchema`, `ResponseFormat`} {
		if strings.Contains(source, forbidden) {
			t.Errorf("exact raw transport contains forbidden side path %q", forbidden)
		}
	}
}
