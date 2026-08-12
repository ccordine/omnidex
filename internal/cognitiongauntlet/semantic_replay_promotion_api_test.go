package cognitiongauntlet

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestSemanticReplayPromotionRequiresPreregisteredAuthority(t *testing.T) {
	if _, err := VerifyProductionSemanticReplayFor(nil, matrixReplayPreregistration{}); err == nil {
		t.Fatal("semantic replay verifier accepted an absent preregistration")
	}
	if (VerifiedProductionSemanticReplay{}).RequireSeriousExecution() == nil {
		t.Fatal("zero semantic replay verification qualified serious execution")
	}
}

func TestProductionSemanticReplayRejectsNonFullPreregistration(t *testing.T) {
	config, registration := replayPreregistrationTestConfig(t, []uint64{101})
	credential, err := loadMatrixReplayPreregistration(
		config, registration.Cases[0].ID, VariantRawObservation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyProductionSemanticReplayFor(nil, credential); err == nil {
		t.Fatal("production replay accepted a non-Full preregistered coordinate")
	}
}

func TestMatrixReplayPreregistrationIsOpaqueAndScheduleDerived(t *testing.T) {
	config, registration := replayPreregistrationTestConfig(t, []uint64{101})
	credential, err := loadMatrixReplayPreregistration(
		config, registration.Cases[0].ID, VariantFullCognition,
	)
	if err != nil || credential.validate() != nil {
		t.Fatalf("load replay preregistration: %v", err)
	}
	typeOf := reflect.TypeOf(credential)
	for index := 0; index < typeOf.NumField(); index++ {
		if typeOf.Field(index).PkgPath == "" {
			t.Fatalf("replay preregistration exposes field %q", typeOf.Field(index).Name)
		}
	}
	raw, err := json.Marshal(credential)
	if err != nil || string(raw) != "{}" {
		t.Fatalf("opaque replay preregistration serialized as %q: %v", raw, err)
	}
	if _, err := loadMatrixReplayPreregistration(
		config, "unregistered-case", VariantFullCognition,
	); err == nil {
		t.Fatal("replay preregistration accepted an unregistered case")
	}
	changed := config
	changed.PreregistrationSHA256 = digestExactBytes([]byte("changed"))
	if _, err := loadMatrixReplayPreregistration(
		changed, registration.Cases[0].ID, VariantFullCognition,
	); err == nil {
		t.Fatal("replay preregistration accepted a changed Matrix seal")
	}
}

func TestMatrixReplayPreregistrationRejectsAnotherCoordinateAndEarlyStart(t *testing.T) {
	config, registration := replayPreregistrationTestConfig(t, []uint64{101, 102})
	first, err := loadMatrixReplayPreregistration(
		config, registration.Cases[0].ID, VariantFullCognition,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadMatrixReplayPreregistration(
		config, registration.Cases[1].ID, VariantFullCognition,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.binds(
		second.authority, second.episode.ID, registration.RegisteredAt.Add(time.Second),
	); err == nil {
		t.Fatal("replay preregistration accepted another Matrix coordinate")
	}
	if err := first.binds(
		first.authority, first.episode.ID, registration.RegisteredAt.Add(-time.Nanosecond),
	); err == nil {
		t.Fatal("replay preregistration accepted an episode started before registration")
	}
	if err := first.binds(
		first.authority, first.episode.ID, registration.RegisteredAt,
	); err != nil {
		t.Fatalf("exact registered replay coordinate was rejected: %v", err)
	}
}

func TestMatrixReplayPreregistrationBindsExactExecutionIdentity(t *testing.T) {
	config, registration := replayPreregistrationTestConfig(t, []uint64{101})
	credential, err := loadMatrixReplayPreregistration(
		config, registration.Cases[0].ID, VariantFullCognition,
	)
	if err != nil {
		t.Fatal(err)
	}
	manifest := EpisodeManifest{
		OmnidexCommit:           config.OmnidexCommit,
		LedgerSchemaVersion:     config.LedgerSchemaVersion,
		WorkingSetPolicyVersion: config.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: config.ProjectionPolicyVersion,
	}
	if err := credential.bindsExecution(manifest); err != nil {
		t.Fatalf("exact preregistered execution identity was rejected: %v", err)
	}
	for name, mutate := range map[string]func(*EpisodeManifest){
		"Omnidex commit": func(value *EpisodeManifest) {
			value.OmnidexCommit = "0123456789abcdef0123456789abcdef01234567"
		},
		"ledger schema": func(value *EpisodeManifest) {
			value.LedgerSchemaVersion += ".changed"
		},
		"Working Set policy": func(value *EpisodeManifest) {
			value.WorkingSetPolicyVersion += ".changed"
		},
		"projection policy": func(value *EpisodeManifest) {
			value.ProjectionPolicyVersion += ".changed"
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := manifest
			mutate(&changed)
			if credential.bindsExecution(changed) == nil {
				t.Fatal("replay credential accepted changed execution identity")
			}
		})
	}
}

func TestSemanticReplayHasNoSelfContainedSeriousPromotionAPI(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate semantic replay promotion test")
	}
	file, err := parser.ParseFile(
		token.NewFileSet(), filepath.Join(filepath.Dir(current), "semantic_replay_verify.go"), nil, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	foundFor := false
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		switch function.Name.Name {
		case "VerifyProductionSemanticReplay", "NewProductionSemanticReplayExpectation":
			t.Fatal("self-contained semantic replay promotion API returned")
		case "VerifyProductionSemanticReplayFor":
			foundFor = true
		}
	}
	if !foundFor {
		t.Fatal("preregistered semantic replay promotion API is absent")
	}
	preregistrationFile, err := parser.ParseFile(
		token.NewFileSet(), filepath.Join(
			filepath.Dir(current), "semantic_replay_preregistration.go",
		), nil, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range preregistrationFile.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok &&
			function.Recv == nil && ast.IsExported(function.Name.Name) {
			t.Fatalf("replay preregistration exports post-run issuer %q", function.Name.Name)
		}
		typeDeclaration, ok := declaration.(*ast.GenDecl)
		if !ok || typeDeclaration.Tok != token.TYPE {
			continue
		}
		for _, specification := range typeDeclaration.Specs {
			named := specification.(*ast.TypeSpec)
			if ast.IsExported(named.Name.Name) {
				t.Fatalf("replay preregistration exports constructible type %q", named.Name.Name)
			}
		}
	}
}

func replayPreregistrationTestConfig(
	t *testing.T,
	seeds []uint64,
) (OfflineMatrixConfig, OfflineMatrixPreregistration) {
	t.Helper()
	base := validOfflineOutputConfig(t)
	plan := OfflineMatrixPlan{
		Policy: CompetenceSuccessSuperiority, Suites: []Suite{SuiteRetrieve},
		Seeds: seeds, Repetitions: 1, Surface: SurfaceFilesystem,
	}
	config := OfflineMatrixConfig{
		Schema: OfflineMatrixConfigSchemaV3, Plan: plan, Budget: base.Scenario.Budget(),
		DatabaseURL: base.DatabaseURL, OllamaEndpoint: base.OllamaEndpoint,
		InferenceTimeoutSeconds: base.InferenceTimeoutSeconds,
		PublicOutputDirectory:   base.PublicOutputDirectory,
		PrivateOutputDirectory:  base.PrivateOutputDirectory,
		RatGeneration:           base.RatGeneration, PreparedBrainEvidence: base.PreparedBrainEvidence,
		RuntimeFingerprint: base.RuntimeFingerprint, OmnidexCommit: base.OmnidexCommit,
		LedgerSchemaVersion:     base.LedgerSchemaVersion,
		WorkingSetPolicyVersion: base.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: base.ProjectionPolicyVersion,
	}
	registration, err := NewOfflineMatrixPreregistration(plan, config.fixedAuthority())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.PrivateOutputDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SealOfflineMatrixPreregistration(config.Paths().Preregistration, registration); err != nil {
		t.Fatal(err)
	}
	config.PreregistrationSHA256, err = registration.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	return config, registration
}
