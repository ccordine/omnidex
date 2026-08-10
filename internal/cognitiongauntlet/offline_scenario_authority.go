package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/labyrinth"
)

func (generated generatedOfflineScenario) pairedAuthority(
	surface Surface,
	rat RatGeneration,
	repetition int,
	runtime RuntimeFingerprint,
) (PairedRunAuthority, error) {
	if err := generated.spec.Validate(); err != nil {
		return PairedRunAuthority{}, err
	}
	if err := generated.scenario.Validate(); err != nil {
		return PairedRunAuthority{}, err
	}
	surfaceVersion, err := surface.Version()
	if err != nil {
		return PairedRunAuthority{}, err
	}
	fixtureVersion := ""
	generatorVersion := ""
	switch generated.spec.Kind {
	case OfflineScenarioInitial:
		oracle := generated.initial.generated.PrivateOracle()
		fixtureVersion = generated.spec.Initial.FixtureVersion
		generatorVersion = oracle.GeneratorVersion
	case OfflineScenarioExtended:
		oracle := generated.extended.PrivateOracle()
		fixtureVersion = generated.spec.Extended.FixtureVersion
		generatorVersion = oracle.GeneratorVersion
	default:
		return PairedRunAuthority{}, fmt.Errorf("offline generated scenario kind is invalid")
	}
	authority := PairedRunAuthority{
		Schema: PairedRunAuthoritySchemaV1,
		CaseID: generated.spec.CaseID(), Suite: generated.suite,
		FixtureVersion: fixtureVersion, GeneratorVersion: generatorVersion,
		Seed: generated.spec.Seed(), Scenario: generated.scenario.Ref(),
		OracleSHA256: generated.oracleSHA256, SurfaceVersion: surfaceVersion,
		ActionCatalogVersion: generated.public.World.Catalog.Version,
		ActionCatalogSHA256:  generated.public.World.Catalog.SHA256,
		RatGeneration:        rat, Budget: generated.spec.Budget(), Runtime: runtime,
		Repetition: repetition,
	}
	return authority, authority.Validate()
}

func (generated generatedOfflineScenario) oracleManifest() (OracleManifest, error) {
	switch generated.spec.Kind {
	case OfflineScenarioInitial:
		return generated.initial.oracleManifest()
	case OfflineScenarioExtended:
		oracle := generated.extended.PrivateOracle()
		cost := int64(0)
		for _, action := range oracle.Witness {
			cost += int64(action.Cost)
		}
		manifest := OracleManifest{
			Schema:     OracleManifestSchemaV1,
			ScenarioID: oracle.ScenarioID, PublicSHA256: oracle.PublicSHA256,
			OracleSHA256: oracle.OracleSHA256, GeneratorVersion: oracle.GeneratorVersion,
			Seed: oracle.Seed, Quality: OracleWitnessOnly,
			WitnessCost: cost, LowerBound: 1, TaskArchetype: oracle.TaskArchetype,
		}
		return manifest, manifest.Validate()
	default:
		return OracleManifest{}, fmt.Errorf("offline generated scenario kind is invalid")
	}
}

func (generated generatedOfflineScenario) evidenceAuthority() offlineEvidenceAuthority {
	switch generated.spec.Kind {
	case OfflineScenarioInitial:
		oracle := generated.initial.generated.PrivateOracle()
		return offlineEvidenceAuthority{
			Scenario: generated.scenario.Ref(), Suite: oracleSuite(generated.suite),
			OracleSHA256: oracle.OracleSHA256, Witness: oracle.Witness,
			EvidenceUses: oracle.EvidenceUses, RequiredEvidence: len(oracle.RequiredEvidence),
		}
	case OfflineScenarioExtended:
		oracle := generated.extended.PrivateOracle()
		return offlineEvidenceAuthority{
			Scenario: generated.scenario.Ref(), Suite: oracle.Suite,
			OracleSHA256: oracle.OracleSHA256, Witness: oracle.Witness,
			EvidenceUses: oracle.EvidenceUses, RequiredEvidence: len(oracle.EvidenceUses),
		}
	default:
		return offlineEvidenceAuthority{}
	}
}

func oracleSuite(suite Suite) labyrinth.Suite { return labyrinth.Suite(suite) }
