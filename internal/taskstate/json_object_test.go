package taskstate

import (
	"fmt"
	"strings"
	"testing"
)

func TestJSONObjectRejectsTrailingDataAndDuplicateKeys(t *testing.T) {
	for _, raw := range []string{
		`{} garbage`,
		`{"a":1,"a":2}`,
		`{"nested":{"x":1,"x":2}}`,
	} {
		if _, err := NewJSONObject([]byte(raw)); err == nil {
			t.Fatalf("invalid object accepted: %s", raw)
		}
	}
	object, err := NewJSONObject([]byte(`{"z":2,"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(object.Bytes()); got != `{"a":1,"z":2}` {
		t.Fatalf("canonical object=%s", got)
	}
}

func TestJSONObjectRejectsPostgreSQLUnstableValues(t *testing.T) {
	invalid := [][]byte{
		[]byte(`{"bad\u0000key":"value"}`),
		[]byte(`{"nested":{"value":"bad\u0000value"}}`),
		[]byte(`{"number":1e2}`),
		[]byte(`{"number":1E-2}`),
		[]byte(`{"number":-0}`),
		[]byte(`{"number":-0.0}`),
	}
	invalidUTF8 := append([]byte(`{"value":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`"}`)...)
	invalid = append(invalid, invalidUTF8)
	for _, raw := range invalid {
		if _, err := NewJSONObject(raw); err == nil {
			t.Fatalf("PostgreSQL-unstable JSON accepted: %q", raw)
		}
	}
}

func TestJSONObjectAcceptsPostgreSQLStablePlainDecimals(t *testing.T) {
	object, err := NewJSONObject([]byte(`{"d":10,"a":1.2300,"c":0.0,"b":-0.01}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(object.Bytes()); got != `{"a":1.2300,"b":-0.01,"c":0.0,"d":10}` {
		t.Fatalf("canonical decimals=%s", got)
	}
}

func TestJSONObjectEnforcesRawCanonicalAndFractionLimits(t *testing.T) {
	if _, err := NewJSONObject([]byte(strings.Repeat(" ", MaxJSONObjectInputBytes+1))); err == nil {
		t.Fatal("oversized raw JSON object accepted")
	}
	canonicalExpansion := []byte(`{"value":"` + strings.Repeat("\u2028", MaxJSONObjectBytes/5) + `"}`)
	if len(canonicalExpansion) > MaxJSONObjectBytes {
		t.Fatalf("test raw JSON unexpectedly exceeds limit: %d", len(canonicalExpansion))
	}
	if _, err := NewJSONObject(canonicalExpansion); err == nil {
		t.Fatal("object whose canonical form exceeds the byte limit was accepted")
	}
	fraction := []byte(`{"number":0.` + strings.Repeat("1", maxPostgresJSONBFractionalDigits+1) + `}`)
	if _, err := NewJSONObject(fraction); err == nil {
		t.Fatal("PostgreSQL-out-of-range fractional number accepted")
	}
}

func TestJSONObjectAcceptsPostgreSQLWhitespaceExpandedCanonicalObject(t *testing.T) {
	pairs := make([]string, 0, 6000)
	for index := 0; index < cap(pairs); index++ {
		pairs = append(pairs, fmt.Sprintf(`"k%d": 0`, index))
	}
	raw := []byte("{" + strings.Join(pairs, ", ") + "}")
	if len(raw) <= MaxJSONObjectBytes || len(raw) > MaxJSONObjectInputBytes {
		t.Fatalf("test object bytes=%d, want (%d,%d]", len(raw), MaxJSONObjectBytes, MaxJSONObjectInputBytes)
	}
	object, err := NewJSONObject(raw)
	if err != nil {
		t.Fatalf("PostgreSQL-expanded object rejected: %v", err)
	}
	if len(object.Bytes()) > MaxJSONObjectBytes {
		t.Fatalf("compact canonical bytes=%d", len(object.Bytes()))
	}
}
