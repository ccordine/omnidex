package cognitiongauntlet

import (
	"context"
	"testing"
)

func TestAblationSemanticReplayRejectsAlternateModelVisibleContext(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	oracle := fixture.generated.PrivateOracle()
	for _, variant := range []Variant{
		VariantRawObservation,
		VariantFullTranscript,
		VariantTranscriptCompacted,
		VariantTaskLedger,
		VariantLedgerWorkingSet,
		VariantLedgerProjection,
		VariantRawShell,
	} {
		variant := variant
		t.Run(string(variant), func(t *testing.T) {
			surface := SurfaceSymbolic
			if variant == VariantRawShell {
				surface = SurfaceFilesystem
			}
			request := ablationTestRequest(
				t, variant, surface, 1,
				&witnessPolicyClient{
					model:   mustRatGeneration(t).Fixed.Brain.Model,
					witness: oracle.Witness, evidenceUses: oracle.EvidenceUses,
				},
			)
			result, err := RunAblation(context.Background(), fixture, request)
			if err != nil {
				t.Fatal(err)
			}
			authority, err := ablationEvidenceAuthorityFromEpisode(result.Episode)
			if err != nil {
				t.Fatal(err)
			}
			evidence, err := loadAblationEvidence(request.EvidenceSealPath, authority)
			if err != nil {
				t.Fatal(err)
			}
			callIndex := 0
			if len(evidence.Root.Calls) > 1 {
				callIndex = 1
			}
			evidence.Root.Calls[callIndex].Projection.Projection.Rendered += "\nforged context"
			if err := verifyAblationSemanticContextDerivation(evidence.Root); err == nil {
				t.Fatal("ablation semantic replay accepted a non-derived model-visible context")
			}
		})
	}
}
