package assemblyline

import "testing"

func TestPortableResponseFramingRegistryIsExhaustive(t *testing.T) {
	natural := map[WorkKind]struct{}{
		WorkApplicationProductContext: {}, WorkApplicationRequirement: {},
		WorkApplicationTargetTree: {}, WorkRepositoryRequirement: {},
		WorkRepositoryGroundedCorrection: {}, WorkContextMinification: {},
		WorkConversationResponse: {}, WorkRoleplayGroundedResponseText: {},
		WorkRoleplayOngoingAction: {}, WorkGroundedAnswerText: {},
		WorkDatabaseEvidenceGap: {}, WorkWebSynthesisParagraph: {},
		WorkWebGroundedSynthesisCorrection: {}, WorkTypeScriptRepairGuidance: {},
		WorkFragmentGeneration: {}, WorkFragmentModification: {},
		WorkFragmentCorrection: {},
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
		} else if kind == WorkResponseCorrection {
			want = portableResponseFramingOriginal
		}
		if framing != want {
			t.Fatalf("response framing for %q=%q want %q", kind, framing, want)
		}
	}
	if _, err := PortableResponseFramingForWorkKind("unknown"); err == nil {
		t.Fatal("unregistered work kind received response framing")
	}
}

func TestResponseCorrectionInheritsOriginalResponseFraming(t *testing.T) {
	single, err := NewApplicationClassificationJob(
		ApplicationClassificationInput{UserRequest: "Describe a command-line utility."},
	)
	if err != nil {
		t.Fatal(err)
	}
	multiline, err := NewConversationResponseJob(ConversationResponseInput{
		Kind: ObjectiveKindAnswer, ExactInstruction: "Explain the supplied concept.",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name string
		job  PortableJob
		want PortableResponseFraming
	}{
		{name: "single line", job: single, want: PortableResponseFramingSingleLine},
		{name: "natural multiline", job: multiline, want: PortableResponseFramingNaturalMultiline},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			correction, err := NewRetainedResponseCorrectionJob(
				fixture.job, "candidate violates its exact value contract", "invalid",
			)
			if err != nil {
				t.Fatal(err)
			}
			got, err := PortableResponseFramingForJob(correction)
			if err != nil {
				t.Fatal(err)
			}
			if got != fixture.want {
				t.Fatalf("correction framing=%q want %q", got, fixture.want)
			}
		})
	}
}
