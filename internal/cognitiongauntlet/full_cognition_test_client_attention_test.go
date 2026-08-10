package cognitiongauntlet

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

func TestContaminatedWitnessCapturesExactAcquisitionWithoutPinningRawEvidence(t *testing.T) {
	t.Parallel()
	acquire := cognition.ActionID("witness-acquire")
	consume := cognition.ActionID("witness-consume")
	client := &witnessPolicyClient{
		witness: []labyrinth.WitnessAction{{ID: acquire}, {ID: consume}},
		evidenceUses: []labyrinth.EvidenceUse{{
			Evidence:            labyrinth.EvidenceIdentity{ID: "record-1", SHA256: fullCognitionTestDigest("record")},
			AcquisitionActionID: acquire, RequiredByActionID: consume,
		}},
		next: 1,
	}
	older := witnessEvidenceRef("older", 1)
	acquired := witnessEvidenceRef("acquired", 2)
	attention, err := client.captureAcquiredEvidence([]cognition.EvidenceRef{older, acquired})
	if err != nil {
		t.Fatal(err)
	}
	if attention == nil || len(attention) != 0 {
		t.Fatalf("test witness pinned raw acquisition evidence: %+v", attention)
	}
	if got := client.consumerEvidence(consume); len(got) != 1 || got[0] != acquired {
		t.Fatalf("consumer evidence=%+v", got)
	}
}

func TestContaminatedWitnessFailsWhenAcquisitionObservationIsOmitted(t *testing.T) {
	t.Parallel()
	client := &witnessPolicyClient{
		witness: []labyrinth.WitnessAction{{ID: "acquire"}, {ID: "consume"}},
		evidenceUses: []labyrinth.EvidenceUse{{
			Evidence:            labyrinth.EvidenceIdentity{ID: "record-1", SHA256: fullCognitionTestDigest("record")},
			AcquisitionActionID: "acquire", RequiredByActionID: "consume",
		}},
		next: 1,
	}
	if _, err := client.captureAcquiredEvidence(nil); err == nil {
		t.Fatal("missing acquisition evidence was accepted")
	}
}

func witnessEvidenceRef(id string, revision uint64) cognition.EvidenceRef {
	return cognition.EvidenceRef{
		ObservationID: cognition.ObservationID(id),
		Revision: cognition.WorldRevision{
			EpisodeID: "episode-witness", Number: revision, SHA256: fullCognitionTestDigest(id + "-revision"),
		},
		SHA256: fullCognitionTestDigest(id + "-content"),
	}
}
