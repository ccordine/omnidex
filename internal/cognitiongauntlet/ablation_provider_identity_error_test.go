package cognitiongauntlet

import (
	"errors"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

func TestDevelopmentAblationPreservesRawProviderIdentityFailure(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		phase AblationProviderIdentityPhase
		call  int
	}{
		{name: "bootstrap", phase: AblationProviderBrainBootstrap, call: 1},
		{name: "process", phase: AblationProviderProcess, call: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			brain := mustRatGeneration(t).Fixed.Brain
			witness := &witnessPolicyClient{model: brain.Model}
			client := &providerIdentityFailureClient{
				witnessPolicyClient: witness, failAt: test.call,
			}
			request := ablationTestRequest(
				t, VariantRawObservation, SurfaceSymbolic, 1, witness,
			)
			request.Client = client
			_, runErr := RunAblation(t.Context(), fixture, request)
			var failure *AblationProviderIdentityError
			if !errors.As(runErr, &failure) || failure.Validate() != nil ||
				failure.Phase() != test.phase || failure.PromotionEligible() {
				t.Fatalf("typed provider failure=%+v error=%v", failure, runErr)
			}
			evidence := client.evidence()
			if evidence.Validate() != nil || len(evidence.Operations) != 5 {
				t.Fatalf("test provider failure evidence=%+v", evidence.Ref)
			}
			if test.phase == AblationProviderBrainBootstrap {
				got, ok := failure.BrainBootstrapFailure()
				if !ok || got.IdentityEvidence.Ref != evidence.Ref || got.Validate() != nil {
					t.Fatal("typed ablation error dropped Brain bootstrap raw evidence")
				}
			} else {
				got, ok := failure.ProviderProcessFailure()
				if !ok || got.IdentityEvidence.Ref != evidence.Ref {
					t.Fatal("typed ablation error dropped provider process raw evidence")
				}
			}
			if witness.calls() != 0 {
				t.Fatalf("provider identity failure consumed %d policy calls", witness.calls())
			}
			if _, err := os.Stat(request.EpisodeSealPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("provider identity failure wrote an episode seal: %v", err)
			}
		})
	}
}

func TestDevelopmentAblationProviderFailureRequiresExactOneTypedResult(t *testing.T) {
	cause := errors.New("provider identity failed")
	for _, failure := range []*AblationProviderIdentityError{
		{phase: AblationProviderBrainBootstrap, cause: cause},
		{
			phase: AblationProviderBrainBootstrap, cause: cause,
			bootstrap: &cognitionpolicy.BrainBootstrapFailure{},
			process:   &cognitionpolicy.ProviderProcessFailure{},
		},
	} {
		if failure.Validate() == nil {
			t.Fatal("ablation provider failure accepted an empty or dual result")
		}
	}
}
