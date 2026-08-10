package cognition

import (
	"fmt"
	"reflect"
	"strings"
)

func (value PlanRevisionMaterialization) Validate() error {
	if value.Schema != PlanRevisionMaterializationSchemaV1 ||
		!strings.HasPrefix(value.ID, "cognition_plan_revision_") ||
		value.ID != "cognition_plan_revision_"+value.SHA256 || !validSHA256(value.SHA256) {
		return fmt.Errorf("%w: descriptor identity is invalid", ErrInvalidPlanRevisionMaterialization)
	}
	if !validSHA256(value.SourceSnapshotSHA256) || !validSHA256(value.SourceDecisionSHA256) ||
		!validSHA256(value.SourceProposalSHA256) || !validSHA256(value.ExpectedGraphSHA256) ||
		!validSHA256(value.ResultGraphSHA256) || value.PreviousGeneration == 0 ||
		value.NextGeneration != value.PreviousGeneration+1 || value.ProposalIndex < 0 ||
		value.ProposalIndex >= MaxLedgerProposals {
		return fmt.Errorf("%w: descriptor authority is invalid", ErrInvalidPlanRevisionMaterialization)
	}
	if err := value.CompletionAuthority.Validate(); err != nil {
		return fmt.Errorf("%w: completion authority: %v", ErrInvalidPlanRevisionMaterialization, err)
	}
	if value.Root.ParentID != "" || len(value.Root.DependsOn) != 1 ||
		value.Root.DependsOn[0] != value.Next.ID || value.Next.ParentID != value.Root.ID ||
		len(value.Next.DependsOn) != 0 || len(value.Next.SupportingRefs) == 0 {
		return fmt.Errorf("%w: descriptor does not encode the fixed root-to-next cutover", ErrInvalidPlanRevisionMaterialization)
	}
	if err := validateIdentity(string(value.ActiveObligationID), "active obligation ID"); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPlanRevisionMaterialization, err)
	}
	if err := validateCanonicalPlanRevisionSpec(value.Root, value.NextGeneration, "root"); err != nil {
		return err
	}
	if err := validateCanonicalPlanRevisionSpec(value.Next, value.NextGeneration, "next"); err != nil {
		return err
	}
	rootCheck, err := value.CompletionAuthority.Resolve(value.Root.Desired)
	if err != nil || rootCheck != value.Root.CompletionCheck {
		return fmt.Errorf("%w: root completion check is not code-authorized", ErrInvalidPlanRevisionMaterialization)
	}
	nextCheck, err := value.CompletionAuthority.Resolve(value.Next.Desired)
	if err != nil || nextCheck != value.Next.CompletionCheck {
		return fmt.Errorf("%w: next completion check is not code-authorized", ErrInvalidPlanRevisionMaterialization)
	}
	wantRoot, err := DeriveObligationID(value.EpisodeID, value.NextGeneration, "", value.Root.Desired, rootCheck)
	if err != nil || wantRoot != value.Root.ID {
		return fmt.Errorf("%w: root identity is not code-derived", ErrInvalidPlanRevisionMaterialization)
	}
	wantNext, err := DeriveObligationID(value.EpisodeID, value.NextGeneration, value.Root.ID, value.Next.Desired, nextCheck)
	if err != nil || wantNext != value.Next.ID {
		return fmt.Errorf("%w: next identity is not code-derived", ErrInvalidPlanRevisionMaterialization)
	}
	proposal, err := canonicalPlanRevisionProposal(PlanRevisionProposal{
		Next: value.Next.Desired, EvidenceRefs: value.Next.SupportingRefs,
	})
	if err != nil || !reflect.DeepEqual(proposal.Next, value.Next.Desired) ||
		!reflect.DeepEqual(proposal.EvidenceRefs, value.Next.SupportingRefs) {
		return fmt.Errorf("%w: proposal semantics are not canonical", ErrInvalidPlanRevisionMaterialization)
	}
	wantProposal, err := planRevisionProposalSHA256(proposal)
	if err != nil || wantProposal != value.SourceProposalSHA256 {
		return fmt.Errorf("%w: proposal hash changed", ErrInvalidPlanRevisionMaterialization)
	}
	want, err := planRevisionMaterializationSHA256(value)
	if err != nil || want != value.SHA256 {
		return fmt.Errorf("%w: descriptor hash changed", ErrInvalidPlanRevisionMaterialization)
	}
	return nil
}

func validateCanonicalPlanRevisionSpec(spec ObligationSpec, generation uint64, label string) error {
	canonical := obligationFromSpec(spec, generation)
	if err := canonical.Validate(); err != nil {
		return fmt.Errorf("%w: %s obligation: %v", ErrInvalidPlanRevisionMaterialization, label, err)
	}
	if !reflect.DeepEqual(canonical.DependsOn, spec.DependsOn) ||
		!reflect.DeepEqual(canonical.SupportingRefs, spec.SupportingRefs) {
		return fmt.Errorf("%w: %s obligation is not canonical", ErrInvalidPlanRevisionMaterialization, label)
	}
	return nil
}
