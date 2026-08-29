package worker

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func assertReplacementEnvelope(
	t *testing.T,
	job assemblyline.PortableJob,
	rejectedPrefix string,
) {
	t.Helper()
	if job.Kind != assemblyline.WorkFragmentGenerationReplacement {
		t.Fatalf("replacement kind=%q", job.Kind)
	}
	var input assemblyline.FragmentGenerationReplacementInput
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		t.Fatal(err)
	}
	if input.Original.Signature == "" {
		t.Fatalf("replacement authority=%+v", input)
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "provider output boundary") ||
		!strings.Contains(prompt, "contained no accepted declaration") ||
		strings.Contains(prompt, rejectedPrefix+" partial-provider-suffix") ||
		strings.Contains(string(job.Payload), "partial-provider-suffix") ||
		strings.Contains(string(job.Payload), "gap_opening_id") ||
		strings.Contains(string(job.Payload), "call_receipt_id") ||
		strings.Contains(string(job.Payload), "output_tokens") ||
		strings.Contains(string(job.Payload), "content_bytes") {
		t.Fatalf("replacement prompt/payload is not bounded: prompt=%q payload=%s", prompt, job.Payload)
	}
}

func assertReplacementOrigin(
	t *testing.T,
	origin queue.StationGapReplacementOrigin,
) {
	t.Helper()
	if origin.GapOpeningID != 41 || origin.CallReceiptID != 43 {
		t.Fatalf("replacement origin=%+v", origin)
	}
}

func rawFragmentOutputLimitFixture() *llm.ExactPreparedOutputLimitReachedError {
	failure := &llm.ExactPreparedOutputLimitReachedError{
		DoneReason: "length", PromptTokens: 124, OutputTokens: 900,
		ContextTokens: 1024, MaxOutputTokens: 1024, ContentBytes: 4096,
	}
	if err := failure.Validate(); err != nil {
		panic(err)
	}
	return failure
}

func fragmentOutputLimitFixture() error {
	failure := &persistedFragmentGenerationOutputLimitFailure{
		Evidence:           *rawFragmentOutputLimitFixture(),
		OriginGapOpeningID: 41, OriginCallReceiptID: 43,
	}
	if err := failure.Validate(); err != nil {
		panic(err)
	}
	return errors.Join(failure, errors.New("partial-provider-suffix was rejected"))
}
