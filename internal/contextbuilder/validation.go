package contextbuilder

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

const (
	maxSpecItems         = 64
	maxSpecBytes         = 1024 * 1024
	maxProjectionRecords = 4096
)

func validOmissionSource(omission Omission) bool {
	switch omission.SourceFreshness {
	case SourceFreshnessValidatedCurrent:
		if authorityRank(omission.Authority) == 0 || omission.Reason == OmittedMissingMaterial {
			return false
		}
	case SourceFreshnessUnresolved:
		if omission.Authority != "" || (omission.Reason != OmittedMissingMaterial && omission.Reason != OmittedRoleNotSelected) {
			return false
		}
	default:
		return false
	}
	return true
}

func validateBuildInput(input BuildInput) (map[workingset.ItemID]Material, error) {
	if err := requireExact(input.WorkID, "work ID", ErrInvalidInput); err != nil {
		return nil, err
	}
	if input.WorkingSet == nil || input.WorkingSet.ID() == "" {
		return nil, fmt.Errorf("%w: initialized working set is required", ErrInvalidInput)
	}
	if err := validateSpec(input.Spec); err != nil {
		return nil, err
	}
	resident := make(map[workingset.ItemID]workingset.Item)
	for _, item := range input.WorkingSet.ResidentItems() {
		resident[item.ID] = item
	}
	materials := make(map[workingset.ItemID]Material, len(input.Materials))
	for _, material := range input.Materials {
		if _, duplicate := materials[material.ItemID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate material for item %q", ErrMaterialMismatch, material.ItemID)
		}
		item, exists := resident[material.ItemID]
		if !exists {
			return nil, fmt.Errorf("%w: item %q is not resident", ErrMaterialMismatch, material.ItemID)
		}
		if err := validateMaterial(item, material); err != nil {
			return nil, err
		}
		materials[material.ItemID] = material
	}
	return materials, nil
}

func validateSpec(spec ContextSpec) error {
	if err := requireExact(spec.Name, "spec name", ErrInvalidSpec); err != nil {
		return err
	}
	if err := requireExact(spec.Version, "spec version", ErrInvalidSpec); err != nil {
		return err
	}
	if err := taskstate.ValidateRef(spec.ScopeRef); err != nil {
		return fmt.Errorf("%w: scope reference: %v", ErrInvalidSpec, err)
	}
	if len(spec.Required) == 0 {
		return fmt.Errorf("%w: at least one required selector is required", ErrInvalidSpec)
	}
	if spec.MaxItems < 1 || spec.MaxItems > maxSpecItems || spec.MaxBytes < 128 || spec.MaxBytes > maxSpecBytes {
		return fmt.Errorf("%w: item or byte ceiling is outside registered bounds", ErrInvalidSpec)
	}
	if spec.MaxAcquisitionRounds < 0 || spec.MaxAcquisitionRounds > 3 {
		return fmt.Errorf("%w: acquisition rounds must be between zero and three", ErrInvalidSpec)
	}
	if err := validateAuthorities(spec.AllowedAuthorities); err != nil {
		return err
	}
	ids := make(map[string]struct{})
	roles := make(map[workingset.Role]struct{})
	for _, group := range []struct {
		selectors []Selector
		required  bool
	}{{spec.Required, true}, {spec.Optional, false}} {
		for _, selector := range group.selectors {
			if err := validateSelector(selector, group.required); err != nil {
				return err
			}
			if _, duplicate := ids[selector.ID]; duplicate {
				return fmt.Errorf("%w: selector ID %q is duplicated", ErrInvalidSpec, selector.ID)
			}
			if _, duplicate := roles[selector.Role]; duplicate {
				return fmt.Errorf("%w: role %q has competing selectors", ErrInvalidSpec, selector.Role)
			}
			ids[selector.ID], roles[selector.Role] = struct{}{}, struct{}{}
		}
	}
	return nil
}

func validateSelector(selector Selector, required bool) error {
	if err := requireExact(selector.ID, "selector ID", ErrInvalidSpec); err != nil {
		return err
	}
	if roleRank(selector.Role) == 0 {
		return fmt.Errorf("%w: selector role %q is not registered", ErrInvalidSpec, selector.Role)
	}
	if selector.MaxItems < 1 || selector.MaxItems > maxSpecItems {
		return fmt.Errorf("%w: selector %q has invalid maximum", ErrInvalidSpec, selector.ID)
	}
	if required && (selector.MinItems < 1 || selector.MinItems > selector.MaxItems) {
		return fmt.Errorf("%w: required selector %q has invalid minimum", ErrInvalidSpec, selector.ID)
	}
	if !required && selector.MinItems != 0 {
		return fmt.Errorf("%w: optional selector %q cannot declare a minimum", ErrInvalidSpec, selector.ID)
	}
	return nil
}

func validateAuthorities(authorities []taskstate.Authority) error {
	if len(authorities) == 0 {
		return fmt.Errorf("%w: at least one allowed authority is required", ErrInvalidSpec)
	}
	seen := make(map[taskstate.Authority]struct{}, len(authorities))
	for _, authority := range authorities {
		if authorityRank(authority) == 0 {
			return fmt.Errorf("%w: authority %q is not registered", ErrInvalidSpec, authority)
		}
		if _, duplicate := seen[authority]; duplicate {
			return fmt.Errorf("%w: authority %q is duplicated", ErrInvalidSpec, authority)
		}
		seen[authority] = struct{}{}
	}
	return nil
}

func validateMaterial(item workingset.Item, material Material) error {
	if err := taskstate.ValidateRef(material.CurrentRef); err != nil {
		return fmt.Errorf("%w: item %q current reference: %v", ErrMaterialMismatch, item.ID, err)
	}
	if material.CurrentRef != item.Ref {
		if material.CurrentRef.URI == item.Ref.URI && material.CurrentRef.Relation == item.Ref.Relation {
			return fmt.Errorf("%w: item %q version or hash changed", ErrStaleReference, item.ID)
		}
		return fmt.Errorf("%w: item %q resolved another reference", ErrMaterialMismatch, item.ID)
	}
	if authorityRank(material.Authority) == 0 {
		return fmt.Errorf("%w: item %q has unknown authority", ErrMaterialMismatch, item.ID)
	}
	if err := validateSourceRefs(material.SourceRefs); err != nil {
		return fmt.Errorf("%w: item %q source refs: %v", ErrMaterialMismatch, item.ID, err)
	}
	if material.Content == "" || !utf8.ValidString(material.Content) || strings.IndexByte(material.Content, 0) >= 0 ||
		material.ByteCost != len([]byte(material.Content)) || material.ByteCost > item.ByteCost {
		return fmt.Errorf("%w: item %q content cost is invalid", ErrMaterialMismatch, item.ID)
	}
	return nil
}

func requireExact(value, field string, sentinel error) error {
	if value == "" || !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 ||
		strings.TrimSpace(value) != value || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return fmt.Errorf("%w: %s must be one nonempty whitespace-free exact value", sentinel, field)
	}
	return nil
}

func authorityAllowed(spec ContextSpec, authority taskstate.Authority) bool {
	for _, allowed := range spec.AllowedAuthorities {
		if allowed == authority {
			return true
		}
	}
	return false
}
