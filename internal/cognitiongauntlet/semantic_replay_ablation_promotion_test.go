package cognitiongauntlet

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func TestAblationSemanticReplayPromotionRequiresPreregisteredAuthority(t *testing.T) {
	if _, err := VerifyAblationSemanticReplayFor(nil, matrixReplayPreregistration{}); err == nil {
		t.Fatal("ablation replay accepted an absent preregistration")
	}
	if (VerifiedAblationSemanticReplay{}).RequireSeriousExecution() == nil {
		t.Fatal("zero ablation replay verification qualified serious execution")
	}
}

func TestAblationSemanticReplayBindsClassAndPreregisteredCoordinate(t *testing.T) {
	config, registration := replayPreregistrationTestConfig(t, []uint64{101})
	for _, variant := range []Variant{VariantRawObservation, VariantRawShell} {
		variant := variant
		t.Run(string(variant), func(t *testing.T) {
			artifact, credential := preregisteredAblationReplayForTest(
				t, config, registration.Cases[0], variant,
			)
			verified, err := VerifyAblationSemanticReplayFor(artifact.Bytes, credential)
			if err != nil {
				t.Fatal(err)
			}
			wantClass := AblationReplaySerious
			if variant == VariantRawShell {
				wantClass = AblationReplayBenchmarkOnly
			}
			if verified.SHA256() != artifact.SHA256 || verified.Class() != wantClass {
				t.Fatalf("verified ablation replay changed authority or class: %+v", verified)
			}
			seriousErr := verified.RequireSeriousExecution()
			if (variant == VariantRawShell) != (seriousErr != nil) {
				t.Fatalf("variant %s serious gate error=%v", variant, seriousErr)
			}
			other := VariantRawObservation
			if variant == VariantRawObservation {
				other = VariantFullTranscript
			}
			wrong, err := loadMatrixReplayPreregistration(config, registration.Cases[0].ID, other)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyAblationSemanticReplayFor(artifact.Bytes, wrong); err == nil {
				t.Fatal("ablation replay accepted another preregistered variant")
			}
		})
	}
}

func preregisteredAblationReplayForTest(
	t *testing.T,
	config OfflineMatrixConfig,
	coordinate OfflineMatrixCase,
	variant Variant,
) (artifact cognitionreplay.Artifact, credential matrixReplayPreregistration) {
	t.Helper()
	run, err := config.derivedRunConfig(coordinate, variant)
	if err != nil {
		t.Fatal(err)
	}
	if run.Scenario.Initial == nil {
		t.Fatal("promotion test requires the registered initial scenario family")
	}
	structural, err := initialMicrogauntletSpec(coordinate.Suite)
	if err != nil {
		t.Fatal(err)
	}
	structural.CaseID = run.Scenario.Initial.CaseID
	structural.Generator.Seed = run.Scenario.Initial.Generator.Seed
	fixture, err := GenerateMicrogauntlet(structural)
	if err != nil {
		t.Fatalf("generate preregistered ablation scenario: %v", err)
	}
	oracle := fixture.generated.PrivateOracle()
	client := &witnessPolicyClient{
		model:   run.RatGeneration.Fixed.Brain.Model,
		witness: oracle.Witness, evidenceUses: oracle.EvidenceUses,
	}
	request := ablationTestRequest(t, variant, run.Surface, run.Repetition, client)
	request.RatGeneration = run.RatGeneration
	request.RuntimeFingerprint = run.RuntimeFingerprint
	request.OmnidexCommit = run.OmnidexCommit
	request.LedgerSchemaVersion = run.LedgerSchemaVersion
	request.WorkingSetPolicyVersion = run.WorkingSetPolicyVersion
	request.ProjectionPolicyVersion = run.ProjectionPolicyVersion
	result, err := RunAblation(context.Background(), fixture, request)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := NewVariantPublicInferenceBundle(fixture, result.Authority, variant)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := ExportAblationSemanticReplay(
		bundle, result.Episode, request.EpisodeSealPath, request.EvidenceSealPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	credential, err = loadMatrixReplayPreregistration(config, coordinate.ID, variant)
	if err != nil {
		t.Fatal(err)
	}
	return replay, credential
}
