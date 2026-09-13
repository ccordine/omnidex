package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestLLMCallRootCannotInheritNativeContext(t *testing.T) {
	t.Parallel()
	for _, kind := range []assemblyline.WorkKind{assemblyline.WorkApplicationClassify, assemblyline.WorkFragmentGeneration} {
		record := exactLLMEvidenceFixture(t, kind, "Resolve one local value.", "value")
		record.Prepared.RetainedContext = []int{17, 28}
		if _, err := normalizeLLMCallOpening(record.LLMCallOpeningRecord); err == nil {
			t.Fatal("an independent station inherited context")
		}
	}
}

func exactLLMEvidenceWithNativeContext(record exactLLMEvidenceFixtureRecord, nativeContext string) exactLLMEvidenceFixtureRecord {
	raw := record.Generation.ProviderResponseCapture
	raw = append(append([]byte(nil), raw[:len(raw)-1]...), []byte(`,"context":`+nativeContext+`}`)...)
	record.Generation.ProviderResponseCapture = raw
	record.Generation.ProviderResponseCapturedBytes = len(raw)
	record.Generation.ProviderResponseBytes = int64(len(raw))
	return record
}
