package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

const PublicInferenceBundleSchemaV1 = "omnidex.cognition-public-inference-bundle.v1"

// PublicInferenceBundle contains everything an isolated inference process may
// know. It cannot represent a generator seed, oracle, witness, or evaluator
// label.
type PublicInferenceBundle struct {
	Schema     string                        `json:"schema"`
	Authority  PublicRunAuthority            `json:"authority"`
	Goal       cognition.GoalExpression      `json:"goal"`
	Completion cognition.CompletionAuthority `json:"completion"`
	Catalog    cognition.ActionCatalog       `json:"action_catalog"`
}

func NewPublicInferenceBundle(
	fixture MicrogauntletCase,
	paired PairedRunAuthority,
) (PublicInferenceBundle, error) {
	return NewVariantPublicInferenceBundle(fixture, paired, VariantFullCognition)
}

func NewVariantPublicInferenceBundle(
	fixture MicrogauntletCase,
	paired PairedRunAuthority,
	variant Variant,
) (PublicInferenceBundle, error) {
	return newScenarioPublicInferenceBundle(
		fixture.SealedEnvironmentScenario(), paired, variant,
	)
}

func newScenarioPublicInferenceBundle(
	scenario labyrinth.Scenario,
	paired PairedRunAuthority,
	variant Variant,
) (PublicInferenceBundle, error) {
	if variant != VariantFullCognition && !executableAblation(variant) {
		return PublicInferenceBundle{}, fmt.Errorf("public inference variant %q is not executable", variant)
	}
	public, err := NewPublicRunAuthority(paired, variant)
	if err != nil {
		return PublicInferenceBundle{}, err
	}
	if scenario.Ref() != public.Scenario {
		return PublicInferenceBundle{}, fmt.Errorf("public inference fixture changed the paired scenario")
	}
	completion, err := labyrinth.NewCompletionAuthority(scenario)
	if err != nil {
		return PublicInferenceBundle{}, err
	}
	bundle := PublicInferenceBundle{
		Schema: PublicInferenceBundleSchemaV1, Authority: public,
		Goal: scenario.Goal(), Completion: completion, Catalog: scenario.Catalog(),
	}
	return bundle, bundle.Validate()
}

func (bundle PublicInferenceBundle) Validate() error {
	if bundle.Schema != PublicInferenceBundleSchemaV1 ||
		(bundle.Authority.Variant != VariantFullCognition && !executableAblation(bundle.Authority.Variant)) {
		return fmt.Errorf("public inference bundle schema or variant is invalid")
	}
	if err := bundle.Authority.Validate(); err != nil {
		return err
	}
	if err := bundle.Goal.Validate(); err != nil {
		return err
	}
	if err := bundle.Completion.Validate(); err != nil {
		return err
	}
	if _, err := bundle.Completion.Resolve(bundle.Goal); err != nil {
		return err
	}
	if err := bundle.Catalog.Validate(); err != nil {
		return err
	}
	if bundle.Catalog.Version != bundle.Authority.ActionCatalogVersion ||
		bundle.Catalog.SHA256 != bundle.Authority.ActionCatalogSHA256 {
		return fmt.Errorf("public inference catalog differs from its sealed authority")
	}
	return nil
}

func SealPublicInferenceBundle(path string, bundle PublicInferenceBundle) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	return sealScenarioArtifact(path, bundle, "public cognition inference bundle")
}

func LoadPublicInferenceBundle(path string) (PublicInferenceBundle, error) {
	var bundle PublicInferenceBundle
	if err := loadScenarioArtifact(path, &bundle, "public cognition inference bundle"); err != nil {
		return PublicInferenceBundle{}, err
	}
	if err := bundle.Validate(); err != nil {
		return PublicInferenceBundle{}, err
	}
	return bundle, nil
}
