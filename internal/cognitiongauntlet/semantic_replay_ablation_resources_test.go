package cognitiongauntlet

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/workingset"
)

func TestAblationWorkingSetPeakIncludesInitialTransition(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	base := &witnessPolicyClient{model: mustRatGeneration(t).Fixed.Brain.Model}
	request := ablationTestRequest(t, VariantLedgerWorkingSet, SurfaceSymbolic, 1, base)
	request.Client = &terminalPolicyClient{
		witnessPolicyClient: base, failure: errors.New("registered provider transport failure"),
	}
	result, err := RunAblation(context.Background(), fixture, request)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := loadAblationEvidence(request.EvidenceSealPath, result.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Root.WorkingSet == nil {
		t.Fatal("first transition did not materialize resident Working Set evidence")
	}
	set, err := workingset.Restore(evidence.Root.WorkingSet.Terminal)
	if err != nil {
		t.Fatal(err)
	}
	want := int64(set.Usage().ResidentBytes)
	if want <= 0 {
		t.Fatal("first transition did not materialize resident Working Set evidence")
	}
	if result.Episode.Manifest.Resources.PeakWorkingSetBytes != want {
		t.Fatalf("peak Working Set bytes=%d, want initial resident usage %d",
			result.Episode.Manifest.Resources.PeakWorkingSetBytes, want)
	}
	bundle, err := NewVariantPublicInferenceBundle(
		fixture, result.Authority, VariantLedgerWorkingSet,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExportAblationSemanticReplay(
		bundle, result.Episode, request.EpisodeSealPath, request.EvidenceSealPath,
	); err != nil {
		t.Fatalf("initial Working Set peak could not reopen: %v", err)
	}
}
