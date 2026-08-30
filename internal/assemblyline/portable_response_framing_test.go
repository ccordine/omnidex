package assemblyline

import "testing"

func TestPortableResponseFramingRegistryIsExhaustive(t *testing.T) {
	natural := map[WorkKind]struct{}{
		WorkApplicationProductContext: {}, WorkApplicationRequirement: {},
		WorkApplicationRequirementCandidateSplit:                    {},
		WorkApplicationRequirementCandidateSplitCorrection:          {},
		WorkApplicationRequirementCandidateResultRelationCorrection: {},
		WorkApplicationTargetTree:                                   {}, WorkRepositoryRequirement: {},
		WorkContextMinification:  {},
		WorkConversationResponse: {}, WorkRoleplayGroundedResponseText: {},
		WorkRoleplayOngoingAction: {}, WorkGroundedAnswerText: {},
		WorkDatabaseEvidenceGap: {}, WorkWebSynthesisParagraph: {},
		WorkTypeScriptRepairGuidance: {},
		WorkFragmentGeneration:       {}, WorkFragmentGenerationReplacement: {},
		WorkFragmentModification: {},
		WorkFragmentCorrection:   {},
	}
	seen := make(map[WorkKind]struct{}, len(AllWorkKinds()))
	for _, kind := range AllWorkKinds() {
		if _, duplicate := seen[kind]; duplicate {
			t.Fatalf("portable work kind %q is registered twice", kind)
		}
		seen[kind] = struct{}{}
		framing, err := PortableResponseFramingForWorkKind(kind)
		if err != nil {
			t.Fatalf("response framing for %q: %v", kind, err)
		}
		want := PortableResponseFramingSingleLine
		if _, multiline := natural[kind]; multiline {
			want = PortableResponseFramingNaturalMultiline
		}
		if framing != want {
			t.Fatalf("response framing for %q=%q want %q", kind, framing, want)
		}
	}
	if _, err := PortableResponseFramingForWorkKind("unknown"); err == nil {
		t.Fatal("unregistered work kind received response framing")
	}
}
