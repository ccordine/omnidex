package llm

import (
	"strings"
	"testing"
)

func TestPreparedGenerationContentAndRawBoundsAreIndependent(t *testing.T) {
	const oldConflatedRawLimit = 16 * 1024 * 1024
	wantRaw := (6 * MaxExactPreparedModelContentBytes) +
		(21 * MaxInferenceContextTokens) + (64 * 1024)
	if MaxExactPreparedModelContentBytes != oldConflatedRawLimit {
		t.Fatalf("decoded content bound=%d, want %d", MaxExactPreparedModelContentBytes, oldConflatedRawLimit)
	}
	if MaxExactPreparedProviderResponseBytes != wantRaw ||
		MaxExactPreparedProviderResponseBytes <= MaxExactPreparedModelContentBytes {
		t.Fatalf(
			"raw provider bound=%d, want independent bound %d",
			MaxExactPreparedProviderResponseBytes, wantRaw,
		)
	}
	if MaxOwnedPreparedGenerationBytes !=
		MaxExactPreparedModelContentBytes+MaxExactPreparedProviderResponseBytes+1 {
		t.Fatalf("owned aggregate bound=%d", MaxOwnedPreparedGenerationBytes)
	}

	// Raw provider evidence larger than the decoded-content authority remains
	// ownable. It is validated and decoded separately before semantic use.
	raw := make([]byte, MaxExactPreparedModelContentBytes+1)
	if _, err := OwnBoundedPreparedGeneration(PreparedGeneration{
		ProviderResponseCapture: raw,
	}); err != nil {
		t.Fatalf("independently bounded raw capture was rejected: %v", err)
	}

	content := strings.Repeat("x", MaxExactPreparedModelContentBytes+1)
	if _, err := OwnBoundedPreparedGeneration(PreparedGeneration{
		Content: content,
	}); err == nil {
		t.Fatal("decoded model content exceeded its independent 16 MiB authority")
	}
}

func TestOutputLimitEvidenceUsesDecodedContentBound(t *testing.T) {
	valid := ExactPreparedOutputLimitReachedError{
		DoneReason: "length", PromptTokens: 1, OutputTokens: 1,
		ContextTokens: MinInferenceContextTokens, MaxOutputTokens: 1,
		ContentBytes: MaxExactPreparedModelContentBytes,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("content at decoded bound was rejected: %v", err)
	}
	valid.ContentBytes++
	if err := valid.Validate(); err == nil {
		t.Fatal("output-limit evidence exceeded decoded model content authority")
	}
}

func TestProviderBodyLimitReceiptUsesRawJSONBoundAndOneByteBoundary(t *testing.T) {
	receipt := PreparedGeneration{
		Schema:                      PreparedGenerationSchemaV1,
		Protocol:                    ExactPreparedProtocolPlainCompletionV4,
		ProviderRequestDisposition:  ProviderRequestDispatched,
		ProviderRequestSHA256:       strings.Repeat("a", 64),
		ProviderHTTPStatus:          200,
		ProviderResponseDisposition: ProviderResponseBodyLimit,
		ProviderContentEncoding: NewProviderContentEncodingEvidence(
			nil, false,
		),
		ProviderResponseCaptureSHA256: strings.Repeat("b", 64),
		ProviderResponseCapturedBytes: MaxExactPreparedProviderResponseBytes + 1,
	}
	if err := receipt.ValidateProviderResponseReceipt(); err != nil {
		t.Fatalf("exact one-byte raw limit boundary was rejected: %v", err)
	}
	receipt.ProviderResponseCapturedBytes++
	if err := receipt.ValidateProviderResponseReceipt(); err == nil {
		t.Fatal("provider receipt exceeded raw JSON limit boundary")
	}
}
