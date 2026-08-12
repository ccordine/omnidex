package cognitiongauntlet

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAblationEvidenceColdVerifierBindsPreregisteredCoordinate(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	oracle := fixture.generated.PrivateOracle()
	request := ablationTestRequest(
		t, VariantRawObservation, SurfaceSymbolic, 1,
		&witnessPolicyClient{
			model:   mustRatGeneration(t).Fixed.Brain.Model,
			witness: oracle.Witness, evidenceUses: oracle.EvidenceUses,
		},
	)
	result, err := RunAblation(context.Background(), fixture, request)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := LoadSealedEpisode(request.EpisodeSealPath)
	if err != nil {
		t.Fatal(err)
	}
	public, err := NewPublicRunAuthority(result.Authority, result.Variant.Variant)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := NewAblationEvidenceExpectation(public, sealed)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyAblationEvidenceFor(
		request.EvidenceSealPath, sealed, expected,
	)
	if err != nil {
		t.Fatal(err)
	}
	if verified.SHA256() != result.Evidence.SHA256 ||
		verified.Class() != AblationReplaySerious ||
		verified.Authority() != result.Evidence {
		t.Fatalf("cold evidence verification changed authority: %+v", verified)
	}
	bundle, err := NewVariantPublicInferenceBundle(
		fixture, result.Authority, VariantRawObservation,
	)
	if err != nil {
		t.Fatal(err)
	}
	parentVerified, err := verifyOfflineInferenceAblationEvidence(
		bundle, sealed, request.EvidenceSealPath,
	)
	if err != nil || parentVerified.SHA256() != verified.SHA256() {
		t.Fatalf("parent cold verification=%+v err=%v", parentVerified, err)
	}

	changed := expected
	changed.EpisodeSealSHA256 = strings.Repeat("f", 64)
	if _, err := VerifyAblationEvidenceFor(
		request.EvidenceSealPath, sealed, changed,
	); err == nil {
		t.Fatal("ablation evidence verifier accepted another episode seal")
	}
	changed = expected
	changed.PublicRunAuthority.Variant = VariantFullTranscript
	if _, err := VerifyAblationEvidenceFor(
		request.EvidenceSealPath, sealed, changed,
	); err == nil {
		t.Fatal("ablation evidence verifier accepted another preregistered coordinate")
	}
	if err := os.Remove(request.EvidenceSealPath); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyOfflineInferenceAblationEvidence(
		bundle, sealed, request.EvidenceSealPath,
	); err == nil {
		t.Fatal("parent accepted a sealed ablation with missing exact evidence")
	}
}

func TestAblationEvidenceColdVerifierPreservesNonSeriousClasses(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []Variant{VariantRawShell, VariantOracleEvidence} {
		variant := variant
		t.Run(string(variant), func(t *testing.T) {
			surface := SurfaceSymbolic
			if variant == VariantRawShell {
				surface = SurfaceFilesystem
			}
			oracle := fixture.generated.PrivateOracle()
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
			sealed, err := LoadSealedEpisode(request.EpisodeSealPath)
			if err != nil {
				t.Fatal(err)
			}
			public, err := NewPublicRunAuthority(result.Authority, variant)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := NewAblationEvidenceExpectation(public, sealed)
			if err != nil {
				t.Fatal(err)
			}
			verified, err := VerifyAblationEvidenceFor(
				request.EvidenceSealPath, sealed, expected,
			)
			if err != nil {
				t.Fatal(err)
			}
			want := AblationReplayBenchmarkOnly
			if variant == VariantOracleEvidence {
				want = AblationReplayContaminated
			}
			if verified.Class() != want {
				t.Fatalf("verified class=%q, want %q", verified.Class(), want)
			}
		})
	}
}

func TestVerifiedAblationEvidenceCannotQualifySeriousExecution(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate ablation evidence verifier test")
	}
	file, err := parser.ParseFile(
		token.NewFileSet(), filepath.Join(filepath.Dir(current), "ablation_evidence_verifier.go"), nil, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv == nil || function.Name.Name != "RequireSeriousExecution" {
			continue
		}
		for _, field := range function.Recv.List {
			if selector, ok := field.Type.(*ast.Ident); ok && selector.Name == "VerifiedAblationEvidence" {
				t.Fatal("exact ablation evidence bypassed the semantic replay qualification gate")
			}
		}
	}
}

func TestOfflineParentColdVerifiesAblationEvidenceBeforeCompletingStep(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate ablation evidence verifier test")
	}
	file, err := parser.ParseFile(
		token.NewFileSet(), filepath.Join(filepath.Dir(current), "offline_execution_inference.go"), nil, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	verifyPosition, completePosition := token.NoPos, token.NoPos
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		identifier, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		switch identifier.Name {
		case "verifyOfflineInferenceAblationEvidence":
			verifyPosition = call.Pos()
		case "completeOfflineInferenceStep":
			completePosition = call.Pos()
		}
		return true
	})
	if verifyPosition == token.NoPos || completePosition == token.NoPos ||
		verifyPosition >= completePosition {
		t.Fatal("offline parent can complete inference before cold evidence verification")
	}
}
