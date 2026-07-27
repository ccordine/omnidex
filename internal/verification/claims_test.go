package verification

import (
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
)

func TestAssessClaimsRejectsGenericOverlapAndMemoryAuthority(t *testing.T) {
	records := []evidence.Record{
		{ID: 1, Kind: evidence.KindMemoryExcerpt, Summary: "The routing overhaul is complete and all tests passed"},
		{ID: 2, Kind: evidence.KindModelJudgment, Summary: "The planner believes routing is complete"},
		{ID: 3, Kind: evidence.KindTestResult, Summary: "Unrelated localization checks passed"},
	}
	assessments := AssessClaims("The routing overhaul is complete and all tests passed.", records, 8)
	if len(assessments) != 1 {
		t.Fatalf("assessments=%#v", assessments)
	}
	if assessments[0].Supported {
		t.Fatalf("memory, model judgment, and generic overlap must not support completion: %#v", assessments[0])
	}
}

func TestAssessClaimsRequiresOneConcreteEvidenceRecord(t *testing.T) {
	records := []evidence.Record{{
		ID:      7,
		Kind:    evidence.KindTestResult,
		Summary: "routing contract unit tests passed",
		Excerpt: "TestRoleRouting and TestCapabilityAudit passed",
	}}
	assessments := AssessClaims("The routing contract unit tests passed.", records, 8)
	if len(assessments) != 1 || !assessments[0].Supported {
		t.Fatalf("concrete matching evidence should support claim: %#v", assessments)
	}
	if len(assessments[0].EvidenceRefs) != 1 || assessments[0].EvidenceRefs[0] != 7 {
		t.Fatalf("support references=%#v, want [7]", assessments[0].EvidenceRefs)
	}
}

func TestAssessClaimsSupportsChineseEvidenceWithoutWhitespaceTokenization(t *testing.T) {
	records := []evidence.Record{{
		ID:      9,
		Kind:    evidence.KindTestResult,
		Summary: "路由契约测试已经全部通过",
	}}
	assessments := AssessClaims("路由契约测试已经全部通过并完成验证。", records, 8)
	if len(assessments) != 1 || !assessments[0].Supported {
		t.Fatalf("Chinese evidence should be tokenized and matched: %#v", assessments)
	}
}
