package cognitiongauntlet

import (
	"strings"
	"testing"
)

func TestRoguePrerequisiteBundleRejectsMissingReorderedAndForgedReceipts(t *testing.T) {
	t.Parallel()
	receipts := make([]RoguePrerequisiteReceipt, 0, len(roguePrerequisites()))
	for _, suite := range roguePrerequisites() {
		source := RogueSourceExtendedRuntime
		switch suite {
		case SuiteResume:
			source = RogueSourceResumeRuntime
		case SuiteScale:
			source = RogueSourceScaleFamily
		case SuiteTransfer:
			source = RogueSourceTransferFamily
		}
		receipt, err := newRoguePrerequisite(
			suite, source, strings.Repeat("a", 64),
			[]string{fullCognitionTestDigest("artifact-" + string(suite))}, strings.Repeat("c", 64),
		)
		if err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, receipt)
	}
	bundle, err := NewRoguePrerequisiteBundle(receipts)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RoguePrerequisiteBundle){
		"missing": func(value *RoguePrerequisiteBundle) {
			value.Receipts = value.Receipts[:len(value.Receipts)-1]
		},
		"reordered": func(value *RoguePrerequisiteBundle) {
			value.Receipts[0], value.Receipts[1] = value.Receipts[1], value.Receipts[0]
		},
		"forged": func(value *RoguePrerequisiteBundle) {
			value.Receipts[0].SealedArtifactSHA256s[0] = strings.Repeat("f", 64)
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := cloneRoguePrerequisiteBundle(bundle)
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("changed Rogue prerequisite bundle was accepted")
			}
		})
	}
}

func cloneRoguePrerequisiteBundle(source RoguePrerequisiteBundle) RoguePrerequisiteBundle {
	result := source
	result.Receipts = append([]RoguePrerequisiteReceipt(nil), source.Receipts...)
	for index := range result.Receipts {
		result.Receipts[index].SealedArtifactSHA256s = append(
			[]string(nil), source.Receipts[index].SealedArtifactSHA256s...,
		)
	}
	return result
}
