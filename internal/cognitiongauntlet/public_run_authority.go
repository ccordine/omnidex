package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

const PublicRunAuthoritySchemaV1 = "omnidex.cognition-public-run-authority.v1"

// PublicRunAuthority is the complete pre-evaluation experiment authority. It
// deliberately cannot represent the generator seed or any oracle field.
type PublicRunAuthority struct {
	Schema               string                `json:"schema"`
	Scenario             cognition.ScenarioRef `json:"scenario"`
	SurfaceVersion       string                `json:"surface_version"`
	ActionCatalogVersion string                `json:"action_catalog_version"`
	ActionCatalogSHA256  string                `json:"action_catalog_sha256"`
	Variant              Variant               `json:"variant"`
	RatGeneration        RatGeneration         `json:"rat_generation"`
	Budget               RunBudget             `json:"budget"`
	Runtime              RuntimeFingerprint    `json:"runtime"`
	Repetition           int                   `json:"repetition"`
}

func NewPublicRunAuthority(
	paired PairedRunAuthority,
	variant Variant,
) (PublicRunAuthority, error) {
	if err := paired.Validate(); err != nil {
		return PublicRunAuthority{}, err
	}
	authority := PublicRunAuthority{
		Schema: PublicRunAuthoritySchemaV1, Scenario: paired.Scenario,
		SurfaceVersion:       paired.SurfaceVersion,
		ActionCatalogVersion: paired.ActionCatalogVersion,
		ActionCatalogSHA256:  paired.ActionCatalogSHA256, Variant: variant,
		RatGeneration: paired.RatGeneration, Budget: paired.Budget,
		Runtime: paired.Runtime, Repetition: paired.Repetition,
	}
	return authority, authority.Validate()
}

func (authority PublicRunAuthority) Validate() error {
	if authority.Schema != PublicRunAuthoritySchemaV1 ||
		!validVariant(authority.Variant) || authority.Repetition <= 0 || authority.Repetition > 10_000 {
		return fmt.Errorf("public cognition run authority identity is invalid")
	}
	for label, value := range map[string]string{
		"public run surface version":        authority.SurfaceVersion,
		"public run action catalog version": authority.ActionCatalogVersion,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	if err := authority.Scenario.Validate(); err != nil {
		return err
	}
	if !validDigest(authority.ActionCatalogSHA256) {
		return fmt.Errorf("public run action catalog hash is invalid")
	}
	if err := authority.RatGeneration.Validate(); err != nil {
		return err
	}
	if err := authority.Budget.ValidateFor(authority.RatGeneration); err != nil {
		return err
	}
	return authority.Runtime.Validate()
}

func publicRunAuthoritySHA256(
	paired PairedRunAuthority,
	variant Variant,
) (string, error) {
	authority, err := NewPublicRunAuthority(paired, variant)
	if err != nil {
		return "", err
	}
	return authority.SHA256()
}

func (authority PublicRunAuthority) SHA256() (string, error) {
	if err := authority.Validate(); err != nil {
		return "", err
	}
	return digestJSON(authority)
}

// ValidatePublicRunAuthorityProjection is called only after inference has
// stopped. It proves that the newly loaded private paired authority projects
// to the exact public authority that was frozen before inference.
func ValidatePublicRunAuthorityProjection(
	paired PairedRunAuthority,
	public PublicRunAuthority,
) error {
	expected, err := NewPublicRunAuthority(paired, public.Variant)
	if err != nil {
		return err
	}
	expectedSHA, err := expected.SHA256()
	if err != nil {
		return err
	}
	publicSHA, err := public.SHA256()
	if err != nil {
		return err
	}
	if expectedSHA != publicSHA {
		return fmt.Errorf("private cognition authority does not project to the sealed public authority")
	}
	return nil
}
