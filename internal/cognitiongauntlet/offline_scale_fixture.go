package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/labyrinth"
)

func newPrivateScaleEvaluationFixture(
	registration OfflineScalePreregistration,
	coordinate OfflineScaleCase,
	family labyrinth.ScaleFamilyDescriptor,
	generated generatedOfflineScenario,
	authority PairedRunAuthority,
) (privateScaleEvaluationFixture, error) {
	if generated.initial == nil {
		return privateScaleEvaluationFixture{}, fmt.Errorf("private Scale fixture lacks an initial scenario")
	}
	fixture := privateScaleEvaluationFixture{
		Schema: privateScaleFixtureSchemaV1, Registration: registration,
		Case: coordinate, Family: family, Authority: authority,
		Oracle: generated.initial.generated.PrivateOracle(),
	}
	return fixture, fixture.Validate()
}

func (fixture privateScaleEvaluationFixture) Validate() error {
	if fixture.Schema != privateScaleFixtureSchemaV1 || fixture.Registration.Validate() != nil ||
		fixture.Family.Validate() != nil || fixture.Authority.Validate() != nil ||
		fixture.Oracle.Validate() != nil {
		return fmt.Errorf("private Scale evaluation fixture authority is invalid")
	}
	if !containsOfflineScaleCase(fixture.Registration.Cases, fixture.Case) ||
		fixture.Authority.CaseID != fixture.Case.ID ||
		fixture.Authority.Suite != SuiteCombined ||
		fixture.Authority.Seed != fixture.Registration.Plan.Seed ||
		fixture.Authority.Repetition != fixture.Case.Repetition ||
		fixture.Authority.SurfaceVersion != "symbolic.v1" ||
		fixture.Authority.RatGeneration != fixture.Registration.Fixed.RatGeneration ||
		fixture.Authority.Budget != fixture.Registration.Fixed.Budget ||
		fixture.Authority.Runtime != fixture.Registration.Fixed.RuntimeFingerprint ||
		fixture.Oracle.ScenarioID != fixture.Authority.Scenario.ID ||
		fixture.Oracle.PublicSHA256 != fixture.Authority.Scenario.SHA256 ||
		fixture.Oracle.OracleSHA256 != fixture.Authority.OracleSHA256 {
		return fmt.Errorf("private Scale evaluation fixture changed its case authority")
	}
	matched := false
	if len(fixture.Family.Cases) != len(fixture.Registration.WorldSizes) {
		return fmt.Errorf("private Scale family changed its coordinate count")
	}
	for index, item := range fixture.Family.Cases {
		if item.WorldSize != fixture.Registration.WorldSizes[index] {
			return fmt.Errorf("private Scale family changed coordinate %d", index+1)
		}
		if item.WorldSize == fixture.Case.WorldSize {
			matched = item.Scenario == fixture.Authority.Scenario
			break
		}
	}
	if !matched || fixture.Family.Suite != labyrinth.SuiteCombined ||
		fixture.Family.GeneratorVersion != fixture.Oracle.GeneratorVersion ||
		fixture.Family.ActionCatalog.Version != fixture.Authority.ActionCatalogVersion ||
		fixture.Family.ActionCatalog.SHA256 != fixture.Authority.ActionCatalogSHA256 {
		return fmt.Errorf("private Scale family changed its selected scenario")
	}
	return nil
}

func (fixture privateScaleEvaluationFixture) regenerate() (generatedOfflineScenario, error) {
	family, err := generateOfflineScaleFamily(fixture.Registration)
	if err != nil {
		return generatedOfflineScenario{}, err
	}
	if !reflect.DeepEqual(family.descriptor, fixture.Family) {
		return generatedOfflineScenario{}, fmt.Errorf("private Scale evaluator regeneration changed the family")
	}
	generated, err := family.scenario(fixture.Registration, fixture.Case)
	if err != nil {
		return generatedOfflineScenario{}, err
	}
	if !reflect.DeepEqual(generated.initial.generated.PrivateOracle(), fixture.Oracle) {
		return generatedOfflineScenario{}, fmt.Errorf("private Scale evaluator regeneration changed the oracle")
	}
	paired, err := generated.pairedAuthority(
		SurfaceSymbolic, fixture.Authority.RatGeneration, fixture.Case.Repetition,
		fixture.Authority.Runtime,
	)
	if err != nil || !reflect.DeepEqual(paired, fixture.Authority) {
		return generatedOfflineScenario{}, fmt.Errorf("private Scale evaluator regeneration changed paired authority")
	}
	return generated, nil
}

func containsOfflineScaleCase(values []OfflineScaleCase, expected OfflineScaleCase) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func sealPrivateScaleFixture(
	path string,
	fixture privateScaleEvaluationFixture,
	credential string,
) error {
	if err := fixture.Validate(); err != nil {
		return err
	}
	return sealCredentialedJSON(path, fixture, credential, "private Scale evaluation fixture")
}

func loadPrivateScaleFixture(
	path string,
	credential string,
) (privateScaleEvaluationFixture, error) {
	var fixture privateScaleEvaluationFixture
	if err := loadCredentialedJSON(
		path, &fixture, credential, "private Scale evaluation fixture",
	); err != nil {
		return privateScaleEvaluationFixture{}, err
	}
	return fixture, fixture.Validate()
}
