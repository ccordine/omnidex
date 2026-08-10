package cognitiongauntlet

import (
	"fmt"
	"reflect"
)

func deriveOfflineScaleAuthority(
	registration OfflineScalePreregistration,
	artifacts []offlineScaleArtifacts,
) (ScaleFamilyAuthority, error) {
	if len(artifacts) != registration.RunCount || len(artifacts) == 0 {
		return ScaleFamilyAuthority{}, fmt.Errorf("offline Scale artifacts are incomplete")
	}
	first := artifacts[0].scaleEvidence
	authority := ScaleFamilyAuthority{
		Schema: ScaleFamilyAuthoritySchemaV1, FamilyID: first.Family.FamilyID,
		TaskSuite: SuiteCombined, FixtureVersion: registration.BaseWorkload.Initial.FixtureVersion,
		SurfaceVersion: "symbolic.v1", ActionCatalogVersion: first.Family.ActionCatalog.Version,
		ActionCatalogSHA256:   first.Family.ActionCatalog.SHA256,
		GoalSHA256:            first.Family.GoalSHA256,
		RelevantSurfaceSHA256: first.Family.RelevantSurfaceSHA256,
		SolutionDepth:         first.SolutionDepth, RelevantEvidenceCount: first.RelevantEvidenceCount,
		SemanticDecisionCount: first.SemanticDecisionCount, Variant: VariantFullCognition,
		RatGeneration: registration.Fixed.RatGeneration, Budget: registration.Fixed.Budget,
		Runtime: registration.Fixed.RuntimeFingerprint,
	}
	if err := validateOfflineScaleAuthority(authority, registration); err != nil {
		return ScaleFamilyAuthority{}, err
	}
	for index, artifact := range artifacts {
		evidence := artifact.scaleEvidence
		if !reflect.DeepEqual(evidence.Family, first.Family) ||
			evidence.RelevantSurfaceBytes != first.RelevantSurfaceBytes ||
			evidence.SolutionDepth != first.SolutionDepth ||
			evidence.RelevantEvidenceCount != first.RelevantEvidenceCount ||
			evidence.SemanticDecisionCount != first.SemanticDecisionCount {
			return ScaleFamilyAuthority{}, fmt.Errorf("offline Scale run %d changed the family", index+1)
		}
	}
	return authority, nil
}

func validateOfflineScaleAuthority(
	authority ScaleFamilyAuthority,
	registration OfflineScalePreregistration,
) error {
	if err := authority.Validate(); err != nil {
		return err
	}
	if authority.TaskSuite != SuiteCombined || authority.Variant != registration.Variant ||
		authority.FixtureVersion != registration.BaseWorkload.Initial.FixtureVersion ||
		authority.SurfaceVersion != "symbolic.v1" ||
		authority.RatGeneration != registration.Fixed.RatGeneration ||
		authority.Budget != registration.Fixed.Budget ||
		authority.Runtime != registration.Fixed.RuntimeFingerprint {
		return fmt.Errorf("offline Scale family changed preregistered task or runtime")
	}
	return nil
}
