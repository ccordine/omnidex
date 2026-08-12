package cognitiongauntlet

import (
	"strings"
	"testing"
)

func TestProductionSemanticReplaySidecarsAreRequiredBeforeQualification(t *testing.T) {
	if (ProductionSemanticReplaySidecars{}).validate() == nil {
		t.Fatal("missing runtime provider sidecars were accepted")
	}
	valid := ProductionSemanticReplaySidecars{
		RuntimeBrainBootstrapEvidence:     []byte(`{"bootstrap":true}`),
		RuntimeProviderActivationEvidence: []byte(`{"activation":true}`),
	}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	valid.RuntimeBrainBootstrapEvidence = make([]byte, maxRuntimeBrainBootstrapEvidenceBytes+1)
	if valid.validate() == nil {
		t.Fatal("oversized runtime provider sidecar was accepted")
	}
}

func TestSemanticReplayEmptyPolicyBodyIsCaptureOnly(t *testing.T) {
	emptySHA := digestExactBytes(nil)
	for _, test := range []struct {
		kind string
		ok   bool
	}{
		{kind: "provider_response_capture", ok: true},
		{kind: "model_response"},
		{kind: "provider_generation"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			metadata := semanticPolicyEvidence{
				EvidenceKind: test.kind, EvidenceID: "evidence-" + test.kind,
				ContentSHA256: emptySHA, Bytes: 0,
			}
			content, bindings, blobs, err := semanticReplayPolicyBodyContent(test.kind, metadata, []byte{})
			if test.ok {
				if err != nil || content.Storage != "empty" || len(bindings) != 0 || len(blobs) != 0 {
					t.Fatalf("empty capture content=%+v bindings=%d blobs=%d err=%v",
						content, len(bindings), len(blobs), err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "cannot be empty") {
				t.Fatalf("empty %s body error=%v", test.kind, err)
			}
		})
	}
}
