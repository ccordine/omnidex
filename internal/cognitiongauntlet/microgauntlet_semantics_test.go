package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

func TestMicrogauntletSuiteMotifsAreCodeValidated(t *testing.T) {
	tests := []struct {
		suite labyrinth.Suite
		kinds []cognition.ActionKind
	}{
		{labyrinth.SuiteRetrieve, []cognition.ActionKind{"search", "read", "navigate", "take"}},
		{labyrinth.SuiteRecall, []cognition.ActionKind{"read", "navigate", "observe", "navigate", "take"}},
		{labyrinth.SuiteUnlock, []cognition.ActionKind{"search", "take", "use", "navigate"}},
		{labyrinth.SuiteMutate, []cognition.ActionKind{"search", "read", "navigate", "write"}},
		{labyrinth.SuiteCombined, []cognition.ActionKind{"search", "take", "use", "observe", "read", "navigate", "write"}},
	}
	for _, test := range tests {
		if err := validateSuiteMotif(test.suite, test.kinds); err != nil {
			t.Fatalf("suite %s: %v", test.suite, err)
		}
	}
}

func TestInitialMicrogauntletsRequireExactCausalEvidenceUses(t *testing.T) {
	fixtures, err := GenerateInitialMicrogauntletsV1()
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		oracle := fixture.generated.PrivateOracle()
		if err := validateEvidenceUseContract(
			fixture.spec.Generator.Suite, oracle, fixture.generated.PublicArtifact(),
		); err != nil {
			t.Fatalf("suite %s: %v", fixture.spec.Generator.Suite, err)
		}
		if len(oracle.EvidenceUses) != fixture.generated.PublicArtifact().World.Descriptor.Difficulty.EvidenceArtifacts {
			t.Fatalf("suite %s evidence uses=%d", fixture.spec.Generator.Suite, len(oracle.EvidenceUses))
		}
	}
}

func TestMicrogauntletAdmissionRejectsNoncausalOrMismatchedEvidenceUse(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	oracle := fixture.generated.PrivateOracle()
	oracle.EvidenceUses[0].AcquisitionActionID = oracle.EvidenceUses[0].RequiredByActionID
	if err := validateEvidenceUseContract(
		fixture.spec.Generator.Suite, oracle, fixture.generated.PublicArtifact(),
	); err == nil {
		t.Fatal("microgauntlet admitted evidence acquired at its consumer")
	}
	oracle = fixture.generated.PrivateOracle()
	oracle.EvidenceUses[0].Evidence.SHA256 = strings.Repeat("f", 64)
	if err := validateEvidenceUseContract(
		fixture.spec.Generator.Suite, oracle, fixture.generated.PublicArtifact(),
	); err == nil {
		t.Fatal("microgauntlet admitted evidence whose hash was absent from the public world")
	}
}

func TestMicrogauntletSuiteMotifsRejectNominalOrMisorderedWitnesses(t *testing.T) {
	if err := validateSuiteMotif(
		labyrinth.SuiteRecall,
		[]cognition.ActionKind{"read", "navigate", "read", "navigate"},
	); err == nil {
		t.Fatal("Recall accepted immediate reuse without an intervening delay")
	}
	if err := validateSuiteMotif(
		labyrinth.SuiteUnlock,
		[]cognition.ActionKind{"search", "use", "take", "navigate"},
	); err == nil {
		t.Fatal("Unlock accepted use before prerequisite acquisition")
	}
	if err := validateSuiteMotif(
		labyrinth.SuiteMutate,
		[]cognition.ActionKind{"search", "read", "write", "navigate"},
	); err == nil {
		t.Fatal("Mutate accepted a non-terminal mutation")
	}
}
