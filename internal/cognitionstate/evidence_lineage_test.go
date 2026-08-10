package cognitionstate

import "testing"

func TestRawEnvironmentEvidenceIsAProjectionLineageRoot(t *testing.T) {
	t.Parallel()
	_, _, observation := attentionTestRuntime(t)
	candidate := evidenceCandidate(EvidenceMaterial{
		Ref: observation.EvidenceRef(), Content: observation.Content,
	}, true)
	if candidate.sourceRefs == nil || len(candidate.sourceRefs) != 0 {
		t.Fatalf("raw evidence source refs = %#v, want explicit empty lineage", candidate.sourceRefs)
	}
	if candidate.ref != evidenceLedgerRef(observation.EvidenceRef()) {
		t.Fatalf("raw evidence ref = %#v, want exact observation ref", candidate.ref)
	}
}
