package cognitiongauntlet

import "testing"

func TestLiveResumeSemanticComparisonAllowsOnlyOneExactProjectionDuplicate(t *testing.T) {
	baseline := testResumeBaseline(t, 7).Semantics
	live := baseline
	live.ProjectionCount++
	live.ProjectionSequenceSHA256 = traceTestDigest("live-with-one-exact-duplicate")
	if !liveResumeSemanticsMatch(live, baseline) {
		t.Fatal("one exact takeover-equivalent projection was rejected")
	}
	changed := live
	changed.ActionSequenceSHA256 = traceTestDigest("changed-action")
	if liveResumeSemanticsMatch(changed, baseline) {
		t.Fatal("changed action semantics were accepted")
	}
	changed = live
	changed.ProjectionCount++
	if liveResumeSemanticsMatch(changed, baseline) {
		t.Fatal("more than one takeover projection was ignored")
	}
	changed = live
	changed.LogicalProjectionSHA256 = traceTestDigest("changed-logical-context")
	if liveResumeSemanticsMatch(changed, baseline) {
		t.Fatal("changed model-visible logical context was accepted")
	}
}
