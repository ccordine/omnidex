package cognitiongauntlet

import (
	"context"
	"strings"
	"testing"
)

func TestTransferRailBindsTwoSurfaceEpisodesToOneLatentTaskAndRuntime(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[4])
	if err != nil {
		t.Fatal(err)
	}
	authority, err := fixture.TransferAuthority(
		[]Surface{SurfaceFilesystem, SurfaceRecord}, mustRatGeneration(t),
		VariantDeterministicOracle, 1,
		transferTestFingerprint(),
	)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]TransferEpisodeResult, 0, 2)
	for _, surface := range []Surface{SurfaceFilesystem, SurfaceRecord} {
		run, err := RunOracleBaseline(context.Background(), fixture, oracleTestRequest(t, surface, 1))
		if err != nil {
			t.Fatalf("surface %s: %v", surface, err)
		}
		bound, err := BindTransferEpisode(authority, run)
		if err != nil {
			t.Fatalf("bind surface %s: %v", surface, err)
		}
		results = append(results, bound)
	}
	report, err := EvaluateTransferRail(authority, results)
	if err != nil {
		t.Fatal(err)
	}
	if report.Gate.Passed {
		t.Fatal("world-oracle surface validation was presented as cognition transfer")
	}
}

func TestTransferRailRejectsMissingSurfaceAndChangedRuntimeAuthority(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	authority, err := fixture.TransferAuthority(
		[]Surface{SurfaceSymbolic, SurfaceRecord}, mustRatGeneration(t),
		VariantDeterministicOracle, 1,
		transferTestFingerprint(),
	)
	if err != nil {
		t.Fatal(err)
	}
	run, err := RunOracleBaseline(context.Background(), fixture, oracleTestRequest(t, SurfaceSymbolic, 1))
	if err != nil {
		t.Fatal(err)
	}
	bound, err := BindTransferEpisode(authority, run)
	if err != nil {
		t.Fatal(err)
	}
	report, err := EvaluateTransferRail(authority, []TransferEpisodeResult{bound})
	if err != nil {
		t.Fatal(err)
	}
	if report.Gate.Passed {
		t.Fatal("transfer gate passed without every held-out surface")
	}

	changed := authority
	changed.Runtime.PromptSHA256 = strings.Repeat("f", 64)
	if _, err := BindTransferEpisode(changed, run); err == nil {
		t.Fatal("transfer episode was rebound after the prompt authority changed")
	}
}

func TestTransferRailPromotesOnlyFullCognitionAcrossEverySurface(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[4])
	if err != nil {
		t.Fatal(err)
	}
	authority, err := fixture.TransferAuthority(
		[]Surface{SurfaceFilesystem, SurfaceRecord}, mustRatGeneration(t),
		VariantFullCognition, 1, transferTestFingerprint(),
	)
	if err != nil {
		t.Fatal(err)
	}
	authoritySHA, err := digestJSON(authority)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]TransferEpisodeResult, len(authority.SurfaceVersions))
	for index, surface := range authority.SurfaceVersions {
		episodeSeal := strings.Repeat(string(rune('a'+index)), 64)
		results[index] = TransferEpisodeResult{
			AuthoritySHA256: authoritySHA, SurfaceVersion: surface,
			Variant: VariantFullCognition, EpisodeSealSHA256: episodeSeal, GoalSuccess: true,
			CleanDeskQualified: true,
			CausalAcquisition: testCausalAcquisitionReport(
				episodeSeal, authority.OracleSHA256, surface,
			),
		}
	}
	report, err := EvaluateTransferRail(authority, results)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Gate.Passed {
		t.Fatalf("full cognition transfer gate=%+v", report.Gate)
	}
	results[0].CausalAcquisition.AcquiredEvidence--
	if _, err := EvaluateTransferRail(authority, results); err == nil {
		t.Fatal("transfer rail accepted terminal success without causal acquisition")
	}
}

func testCausalAcquisitionReport(
	episodeSeal string,
	oracleSHA string,
	surface string,
) CausalAcquisitionReport {
	return CausalAcquisitionReport{
		Schema: CausalAcquisitionReportSchemaV1, EpisodeSealSHA256: episodeSeal,
		OracleSHA256: oracleSHA, SurfaceVersion: surface,
		EvidenceUseSHA256: strings.Repeat("9", 64), RequiredEvidence: 3, AcquiredEvidence: 3,
		AcquisitionTraceRefs: []string{"observation-acquisition"},
	}
}

func transferTestFingerprint() RuntimeFingerprint {
	return RuntimeFingerprint{
		ProductionSourceSHA256: strings.Repeat("1", 64),
		RendererSHA256:         strings.Repeat("2", 64),
		RetentionPolicySHA256:  strings.Repeat("3", 64),
		ObligationPolicySHA256: strings.Repeat("4", 64),
		PromptSHA256:           strings.Repeat("5", 64),
	}
}
