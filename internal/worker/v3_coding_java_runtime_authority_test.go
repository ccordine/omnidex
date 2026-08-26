package worker

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestJavaRuntimeAuthorityIsExactForFeatureAndAcceptanceGeneration(t *testing.T) {
	t.Parallel()
	specification, workload := javaCommandLineStackFixture(t)
	target := assemblyline.TargetTree{
		StackID: genericJavaCommandLineAdapter, VersionProfileID: javaCommandLineVersionProfileV1,
		Paths: []string{"Echo.java", "EchoTest.java"},
	}
	coverage, err := assemblyline.NewApplicationFileCoveragePlan(
		workload, target,
		map[string][]string{
			"Echo.java": {workload.Tasks[0].ID}, "EchoTest.java": {workload.Tasks[0].ID},
		},
		map[string]assemblyline.TargetArtifactKind{
			"Echo.java":     assemblyline.TargetArtifactImplementation,
			"EchoTest.java": assemblyline.TargetArtifactVerification,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	blueprint, _, err := compileGenericJavaCommandLineBlueprint(
		"java-authority", specification, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
	)
	if err != nil {
		t.Fatal(err)
	}
	feature, acceptance := javaGeneratedRoleRefs(t, blueprint, workload.Tasks[0].ID)
	stage := directCodingProgram{
		StackID: genericJavaCommandLineAdapter, VersionProfileID: javaCommandLineVersionProfileV1,
		Source: blueprint,
		Generated: map[string]string{
			feature.Block.ID: feature.Block.Signature + " { return null; }",
		},
	}
	featureInput, err := directCodingLanguageFragmentInput(&stage, feature, "java")
	if err != nil {
		t.Fatal(err)
	}
	acceptanceInput, err := directCodingLanguageFragmentInput(&stage, acceptance, "java")
	if err != nil {
		t.Fatal(err)
	}

	wantFeature := append([]string{
		javaCommandLineRuntimeFeatureResultAPI(),
	}, javaCommandLineFragmentGlobals()...)
	if !reflect.DeepEqual(featureInput.PermittedSymbols, wantFeature) {
		t.Fatalf("Java feature permitted symbols=%q want=%q", featureInput.PermittedSymbols, wantFeature)
	}
	wantAcceptance := append([]string{
		javaCommandLineRuntimeAcceptanceAssertAPI(), feature.Block.API,
	}, javaCommandLineFragmentGlobals()...)
	if !reflect.DeepEqual(acceptanceInput.PermittedSymbols, wantAcceptance) {
		t.Fatalf(
			"Java acceptance permitted symbols=%q want=%q",
			acceptanceInput.PermittedSymbols, wantAcceptance,
		)
	}
	for _, projection := range append(
		append([]string(nil), featureInput.PermittedSymbols...),
		acceptanceInput.PermittedSymbols...,
	) {
		if strings.Contains(projection, ") {") {
			t.Fatalf("Java generation received an API method body: %s", projection)
		}
	}

	featureVisible := strings.Join(featureInput.PermittedSymbols, "\n")
	if strings.Contains(featureVisible, "requireResult") || strings.Contains(featureVisible, "require(") {
		t.Fatalf("Java feature received assertion-only runtime authority: %s", featureVisible)
	}
	acceptanceVisible := strings.Join(acceptanceInput.PermittedSymbols, "\n")
	for _, unavailable := range []string{
		" result(", " dependency(", " output(", " error(", " exitCode(", " state(",
	} {
		if strings.Contains(acceptanceVisible, unavailable) {
			t.Fatalf("Java acceptance received unrelated runtime helper %q", unavailable)
		}
	}
	for _, api := range []string{
		javaCommandLineRuntimeFeatureResultAPI(),
		javaCommandLineRuntimeAcceptanceAssertAPI(),
		javaCommandLineRuntimeApplicationInspectAPI(),
	} {
		assertJavaRuntimeAPIDeclarationsOnly(t, api)
	}
}

func TestJavaRuntimeSourcePartitionPreservesCodeOwnedImplementation(t *testing.T) {
	t.Parallel()
	document := javaCommandLineRuntimeDocument()
	parts := make([]string, 0, len(document.Blocks))
	for _, block := range document.Blocks {
		parts = append(parts, block.Static)
	}
	if got := strings.Join(parts, "\n\n"); got != javaCommandLineRuntimeSource() {
		t.Fatal("Java runtime API partition changed the code-owned implementation")
	}
}

func javaGeneratedRoleRefs(
	t *testing.T,
	blueprint assemblyline.SourceBlueprint,
	taskID string,
) (assemblyline.SourceBlockRef, assemblyline.SourceBlockRef) {
	t.Helper()
	refs, err := directCodingTaskGeneratedBlockRefs(blueprint, taskID)
	if err != nil {
		t.Fatal(err)
	}
	var feature, acceptance assemblyline.SourceBlockRef
	for _, ref := range refs {
		switch ref.Block.Role {
		case assemblyline.SourceBlockTaskImplementation:
			feature = ref
		case assemblyline.SourceBlockTaskVerification:
			acceptance = ref
		}
	}
	if feature.Block.ID == "" || acceptance.Block.ID == "" {
		t.Fatalf("Java generated roles feature=%q acceptance=%q", feature.Block.ID, acceptance.Block.ID)
	}
	return feature, acceptance
}

func assertJavaRuntimeAPIDeclarationsOnly(t *testing.T, api string) {
	t.Helper()
	lines := strings.Split(api, "\n")
	if len(lines) < 3 || lines[0] != "final class Runtime {" || lines[len(lines)-1] != "}" {
		t.Fatalf("Java runtime API is not one exact class declaration: %q", api)
	}
	for _, line := range lines[1 : len(lines)-1] {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "static native ") || !strings.HasSuffix(trimmed, ";") {
			t.Fatalf("Java runtime API exposed non-declaration authority: %q", line)
		}
	}
}
