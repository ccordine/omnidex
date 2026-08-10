package llm

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestProviderContentEncodingEvidencePreservesEveryAdmittedHeaderShape(t *testing.T) {
	t.Parallel()
	invalidUTF8 := string([]byte{0xff, 0x00, 'x'})
	for _, testCase := range []struct {
		name       string
		values     []string
		compressed bool
		identity   bool
	}{
		{name: "absent", identity: true},
		{name: "explicit identity", values: []string{"identity"}, identity: true},
		{name: "gzip", values: []string{"gzip"}},
		{name: "empty value", values: []string{""}},
		{name: "multiple values", values: []string{"identity", "gzip"}},
		{name: "invalid string bytes", values: []string{invalidUTF8}},
		{name: "transport decompressed", compressed: true},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			evidence := NewProviderContentEncodingEvidence(testCase.values, testCase.compressed)
			if err := evidence.Validate(); err != nil {
				t.Fatal(err)
			}
			if evidence.IsIdentity() != testCase.identity {
				t.Fatalf("identity=%t want %t", evidence.IsIdentity(), testCase.identity)
			}
		})
	}
}

func TestProviderContentEncodingEvidenceUsesExactBoundedOverflowWitness(t *testing.T) {
	t.Parallel()
	evidence := NewProviderContentEncodingEvidence(
		[]string{strings.Repeat("x", MaxProviderContentEncodingCaptureBytes+1)}, false,
	)
	if err := evidence.Validate(); err != nil {
		t.Fatal(err)
	}
	if evidence.Complete || evidence.CapturedBytes != MaxProviderContentEncodingCaptureBytes ||
		evidence.Bytes <= int64(evidence.CapturedBytes) || evidence.IsIdentity() {
		t.Fatalf("overflow receipt is not exact: %+v", evidence)
	}

	mutations := map[string]func(*ProviderContentEncodingEvidence){
		"short prefix": func(value *ProviderContentEncodingEvidence) {
			value.CapturedBase64 = base64.StdEncoding.EncodeToString(
				make([]byte, MaxProviderContentEncodingCaptureBytes-1),
			)
			value.CapturedBytes--
		},
		"forged complete": func(value *ProviderContentEncodingEvidence) { value.Complete = true },
		"short total": func(value *ProviderContentEncodingEvidence) {
			value.Bytes = MaxProviderContentEncodingCaptureBytes - 1
		},
		"equal total": func(value *ProviderContentEncodingEvidence) {
			value.Bytes = int64(value.CapturedBytes)
		},
		"zero values": func(value *ProviderContentEncodingEvidence) { value.Values = 0 },
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			changed := evidence
			mutate(&changed)
			if changed.Validate() == nil {
				t.Fatal("mutated overflow receipt validated")
			}
		})
	}
}

func TestProviderContentEncodingEvidenceCaptureBoundary(t *testing.T) {
	t.Parallel()
	for _, total := range []int{
		MaxProviderContentEncodingCaptureBytes - 1,
		MaxProviderContentEncodingCaptureBytes,
		MaxProviderContentEncodingCaptureBytes + 1,
	} {
		evidence := NewProviderContentEncodingEvidence(
			[]string{strings.Repeat("x", total-8)}, false,
		)
		if err := evidence.Validate(); err != nil {
			t.Fatalf("total=%d: %v", total, err)
		}
		if evidence.Complete != (total <= MaxProviderContentEncodingCaptureBytes) {
			t.Fatalf("total=%d complete=%t", total, evidence.Complete)
		}
	}
}
