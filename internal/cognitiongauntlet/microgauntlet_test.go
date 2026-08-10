package cognitiongauntlet

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestInitialMicrogauntletsFreezeFiveDistinctSolvableSuites(t *testing.T) {
	specs := InitialMicrogauntletsV1()
	if len(specs) != 5 {
		t.Fatalf("initial microgauntlets=%d want=5", len(specs))
	}
	want := []Suite{SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined}
	seenSeeds := map[uint64]struct{}{}
	for index, spec := range specs {
		if err := spec.Validate(); err != nil {
			t.Fatalf("spec %d: %v", index, err)
		}
		if Suite(spec.Generator.Suite) != want[index] {
			t.Fatalf("spec %d suite=%q want=%q", index, spec.Generator.Suite, want[index])
		}
		if _, duplicate := seenSeeds[spec.Generator.Seed]; duplicate {
			t.Fatalf("seed %d is duplicated", spec.Generator.Seed)
		}
		seenSeeds[spec.Generator.Seed] = struct{}{}
	}

	fixtures, err := GenerateInitialMicrogauntletsV1()
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != len(specs) {
		t.Fatalf("fixtures=%d want=%d", len(fixtures), len(specs))
	}
	for index, fixture := range fixtures {
		public, err := fixture.PublicManifest(SurfaceSymbolic)
		if err != nil {
			t.Fatalf("fixture %d public manifest: %v", index, err)
		}
		oracle, err := fixture.oracleManifest()
		if err != nil {
			t.Fatalf("fixture %d oracle manifest: %v", index, err)
		}
		if err := ValidateManifestPair(public, oracle); err != nil {
			t.Fatalf("fixture %d authority: %v", index, err)
		}
	}
}

func TestGeneratedPublicMicrogauntletNeverSerializesPrivateAuthority(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[4])
	if err != nil {
		t.Fatal(err)
	}
	artifact := fixture.PublicArtifact()
	raw, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range [][]byte{
		[]byte(`"seed"`), []byte(`"oracle_sha256"`), []byte(`"witness"`),
		[]byte(`"required_evidence"`), []byte(`"task_archetype"`),
	} {
		if bytes.Contains(bytes.ToLower(raw), forbidden) {
			t.Fatalf("public fixture leaked %s", forbidden)
		}
	}
}

func TestMicrogauntletRejectsUnregisteredFixtureAndInsufficientBudget(t *testing.T) {
	spec := InitialMicrogauntletsV1()[0]
	spec.FixtureVersion = "microgauntlets.live-edit"
	if err := spec.Validate(); err == nil {
		t.Fatal("unregistered fixture version was accepted")
	}
	spec = InitialMicrogauntletsV1()[0]
	spec.Budget.EnvironmentActions = spec.Generator.Difficulty.SolutionDepth - 1
	if err := spec.Validate(); err == nil {
		t.Fatal("budget below witness depth was accepted")
	}
	spec = InitialMicrogauntletsV1()[0]
	spec.Budget.Station.MaxInputTokens = 0
	if err := spec.Validate(); err == nil {
		t.Fatal("microgauntlet accepted an unset per-call station budget")
	}

	first := InitialMicrogauntletsV1()
	first[0].CaseID = "changed"
	if strings.HasPrefix(InitialMicrogauntletsV1()[0].CaseID, "changed") {
		t.Fatal("callers mutated the frozen fixture catalog")
	}
}
