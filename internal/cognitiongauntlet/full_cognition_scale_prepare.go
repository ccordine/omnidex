package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/labyrinth"
)

func prepareFullCognitionScale(
	base MicrogauntletCase,
	request FullCognitionScaleRequest,
) ([]MicrogauntletCase, labyrinth.ScaleFamilyDescriptor, ScaleFamilyAuthority, error) {
	if err := base.spec.Validate(); err != nil {
		return nil, labyrinth.ScaleFamilyDescriptor{}, ScaleFamilyAuthority{}, err
	}
	generated, family, err := labyrinth.GenerateScaleFamily(base.spec.Generator, request.WorldSizes)
	if err != nil {
		return nil, labyrinth.ScaleFamilyDescriptor{}, ScaleFamilyAuthority{}, err
	}
	if len(request.Cases) != len(generated) {
		return nil, labyrinth.ScaleFamilyDescriptor{}, ScaleFamilyAuthority{}, fmt.Errorf(
			"full cognition scale requires one request group per generated world",
		)
	}
	fixtures := make([]MicrogauntletCase, len(generated))
	for index := range generated {
		if request.Cases[index].WorldSize != family.Cases[index].WorldSize {
			return nil, labyrinth.ScaleFamilyDescriptor{}, ScaleFamilyAuthority{}, fmt.Errorf(
				"full cognition scale request %d changed generated world size", index+1,
			)
		}
		spec := base.spec
		spec.CaseID = fmt.Sprintf("%s-scale-%d", base.spec.CaseID, family.Cases[index].WorldSize)
		fixtures[index] = MicrogauntletCase{spec: spec, generated: generated[index]}
		if _, err := fixtures[index].PublicManifest(SurfaceSymbolic); err != nil {
			return nil, labyrinth.ScaleFamilyDescriptor{}, ScaleFamilyAuthority{}, err
		}
	}
	authority, err := newScaleFamilyAuthority(base, family, request.Cases)
	return fixtures, family, authority, err
}

func newScaleFamilyAuthority(
	base MicrogauntletCase,
	family labyrinth.ScaleFamilyDescriptor,
	requests []FullCognitionScaleCaseRequest,
) (ScaleFamilyAuthority, error) {
	if len(requests) == 0 || len(requests[0].Runs) == 0 {
		return ScaleFamilyAuthority{}, fmt.Errorf("full cognition scale requires measured repetitions")
	}
	first := requests[0].Runs[0]
	suite, err := gauntletSuite(family.Suite)
	if err != nil {
		return ScaleFamilyAuthority{}, err
	}
	authority := ScaleFamilyAuthority{
		Schema: ScaleFamilyAuthoritySchemaV1, FamilyID: family.FamilyID,
		TaskSuite: suite, FixtureVersion: base.spec.FixtureVersion,
		SurfaceVersion: "symbolic.v1", ActionCatalogVersion: family.ActionCatalog.Version,
		ActionCatalogSHA256: family.ActionCatalog.SHA256, GoalSHA256: family.GoalSHA256,
		RelevantSurfaceSHA256: family.RelevantSurfaceSHA256,
		SolutionDepth:         base.spec.Generator.Difficulty.SolutionDepth,
		RelevantEvidenceCount: len(base.generated.PrivateOracle().RequiredEvidence),
		SemanticDecisionCount: len(base.generated.PrivateOracle().Witness),
		Variant:               VariantFullCognition, RatGeneration: first.RatGeneration,
		Budget: base.spec.Budget, Runtime: first.RuntimeFingerprint,
	}
	if err := authority.Validate(); err != nil {
		return ScaleFamilyAuthority{}, err
	}
	if err := validateFullCognitionScaleRequests(requests, first); err != nil {
		return ScaleFamilyAuthority{}, err
	}
	return authority, nil
}
