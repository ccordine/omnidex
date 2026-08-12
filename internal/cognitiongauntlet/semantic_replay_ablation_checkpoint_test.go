package cognitiongauntlet

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func TestAblationSemanticReplayBoundsDeepestRegisteredCheckpoints(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[4])
	if err != nil {
		t.Fatal(err)
	}
	oracle := fixture.generated.PrivateOracle()
	for _, variant := range []Variant{
		VariantRawObservation, VariantFullTranscript, VariantTranscriptCompacted,
		VariantTaskLedger, VariantLedgerWorkingSet, VariantLedgerProjection,
	} {
		variant := variant
		t.Run(string(variant), func(t *testing.T) {
			request := ablationTestRequest(
				t, variant, SurfaceSymbolic, 1,
				&witnessPolicyClient{
					model:   mustRatGeneration(t).Fixed.Brain.Model,
					witness: oracle.Witness, evidenceUses: oracle.EvidenceUses,
				},
			)
			result, err := RunAblation(context.Background(), fixture, request)
			if err != nil {
				t.Fatal(err)
			}
			bundle, err := NewVariantPublicInferenceBundle(fixture, result.Authority, variant)
			if err != nil {
				t.Fatal(err)
			}
			artifact, err := ExportAblationSemanticReplay(
				bundle, result.Episode, request.EpisodeSealPath, request.EvidenceSealPath,
			)
			if err != nil {
				t.Fatal(err)
			}
			verified, err := cognitionreplay.VerifyBase(artifact.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			checkpoints := verified.Checkpoints()
			for index := 1; index < len(checkpoints); index++ {
				gap := checkpoints[index].AfterEvent - checkpoints[index-1].AfterEvent
				if gap == 0 || gap > cognitionreplay.MaxCheckpointInterval {
					t.Fatalf("checkpoint %d gap=%d", index+1, gap)
				}
			}
			if checkpoints[len(checkpoints)-1].AfterEvent != uint64(len(verified.Events())) {
				t.Fatal("final checkpoint does not cover every semantic event")
			}
		})
	}
}
