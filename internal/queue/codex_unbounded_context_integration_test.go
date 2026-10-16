package queue

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

func TestPostgresPersistsLargeCodexPromptAndNativeUsageWithoutForeignCeilings(t *testing.T) {
	_, repository := freshEvidenceRepository(t, evidenceDatabaseURL(t))
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Retain the exact native context.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "native-context-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim: %v", err)
	}
	record := nativeCodexEvidenceFixture(t, "Required result.", "native-context-turn")
	record.Authority = claim.Authority
	record.Prepared.Prompt = strings.Repeat("Required native context. ", 7000)
	raw := bytes.Replace(record.Generation.ProviderResponseCapture, []byte(`"inputTokens":40`), []byte(`"inputTokens":17000`), 1)
	raw = bytes.Replace(raw, []byte(`"totalTokens":52`), []byte(`"totalTokens":17012`), 1)
	record.Generation.ProviderResponseCapture = raw
	record.Generation.ProviderResponseBytes = int64(len(raw))
	record.Generation.ProviderResponseCapturedBytes = len(raw)
	decoded, err := llm.DecodeExactPreparedResponseForProtocol(record.Prepared.Protocol, 0, raw)
	if err != nil {
		t.Fatal(err)
	}
	record.Generation.Usage = decoded.Usage
	stored, err := recordExactLLMEvidenceFixture(ctx, repository, record)
	if err != nil {
		t.Fatal(err)
	}
	read, err := repository.GetLLMCallEvidence(ctx, stored.ID)
	if err != nil || read.ModelInput != record.Prepared.Prompt || read.PromptTokens != 17000 {
		t.Fatalf("large native evidence rejected or altered: %v", err)
	}
}
