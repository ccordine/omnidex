package contextbuilder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/taskstate"
)

type renderedEnvelope struct {
	Schema      string         `json:"schema"`
	SpecName    string         `json:"spec_name"`
	SpecVersion string         `json:"spec_version"`
	ScopeRef    taskstate.Ref  `json:"scope_ref"`
	Items       []renderedItem `json:"items"`
}

type renderedItem struct {
	Ref       taskstate.Ref `json:"ref"`
	Role      string        `json:"role"`
	Authority string        `json:"authority"`
	Content   string        `json:"content"`
}

func renderMaterials(spec ContextSpec, materials []selectedMaterial) (string, error) {
	items := make([]renderedItem, 0, len(materials))
	for _, selected := range materials {
		items = append(items, renderedItem{
			Ref: selected.item.Ref, Role: string(selected.item.Role),
			Authority: string(selected.material.Authority), Content: selected.material.Content,
		})
	}
	raw, err := json.Marshal(renderedEnvelope{
		Schema: RendererJSONV1, SpecName: spec.Name, SpecVersion: spec.Version,
		ScopeRef: spec.ScopeRef, Items: items,
	})
	if err != nil {
		return "", fmt.Errorf("render context material: %w", err)
	}
	if len(raw) > spec.MaxBytes {
		return "", fmt.Errorf("rendered context is %d bytes and exceeds %d", len(raw), spec.MaxBytes)
	}
	return string(raw), nil
}

func (state *buildState) projection() (Projection, error) {
	rendered, err := renderMaterials(state.input.Spec, state.selected)
	if err != nil {
		return Projection{}, err
	}
	selections := make([]Selection, 0, len(state.selected))
	for _, selected := range state.selected {
		selections = append(selections, Selection{
			ItemID: selected.item.ID, Ref: selected.item.Ref, Role: selected.item.Role,
			SourceRefs: cloneSourceRefs(selected.material.SourceRefs),
			Authority:  selected.material.Authority, SourceFreshness: SourceFreshnessValidatedCurrent,
			ContentSHA256: digestString(selected.material.Content),
			RenderedBytes: selected.material.ByteCost,
		})
	}
	omissions := make([]Omission, 0, len(state.omissions))
	for _, omission := range state.omissions {
		omissions = append(omissions, omission)
	}
	sort.Slice(omissions, func(left, right int) bool {
		if roleRank(omissions[left].Role) != roleRank(omissions[right].Role) {
			return roleRank(omissions[left].Role) < roleRank(omissions[right].Role)
		}
		return omissions[left].ItemID < omissions[right].ItemID
	})
	specSHA, err := digestJSON(state.input.Spec)
	if err != nil {
		return Projection{}, err
	}
	projection := Projection{
		Schema: ProjectionSchemaV1, WorkID: state.input.WorkID,
		SpecName: state.input.Spec.Name, SpecVersion: state.input.Spec.Version,
		SpecSHA256: specSHA, RendererVersion: RendererJSONV1,
		ScopeRef:     state.input.Spec.ScopeRef,
		WorkingSetID: state.input.WorkingSet.ID(), WorkingSetVersion: state.input.WorkingSet.Version(),
		Selected: selections, Omitted: omissions,
		Rendered: rendered, RenderedSHA256: digestString(rendered), RenderedBytes: len([]byte(rendered)),
		EstimatedTokens: (len([]byte(rendered)) + 3) / 4, TokenEstimator: TokenEstimatorV1,
	}
	id, err := projectionID(projection)
	if err != nil {
		return Projection{}, err
	}
	projection.ID = id
	if err := projection.Validate(); err != nil {
		return Projection{}, err
	}
	return projection, nil
}

func (projection Projection) Validate() error {
	if projection.Schema != ProjectionSchemaV1 || !validProjectionID(projection.ID) {
		return fmt.Errorf("%w: schema or identity is invalid", ErrInvalidProjection)
	}
	if err := requireExact(projection.WorkID, "projection work ID", ErrInvalidProjection); err != nil {
		return err
	}
	if err := requireExact(projection.SpecName, "projection spec name", ErrInvalidProjection); err != nil {
		return err
	}
	if err := requireExact(projection.SpecVersion, "projection spec version", ErrInvalidProjection); err != nil {
		return err
	}
	if !validDigest(projection.SpecSHA256) {
		return fmt.Errorf("%w: spec or working-set identity is invalid", ErrInvalidProjection)
	}
	if err := requireExact(string(projection.WorkingSetID), "projection working-set ID", ErrInvalidProjection); err != nil {
		return err
	}
	if err := taskstate.ValidateRef(projection.ScopeRef); err != nil {
		return fmt.Errorf("%w: scope reference is invalid", ErrInvalidProjection)
	}
	if projection.RendererVersion != RendererJSONV1 || projection.TokenEstimator != TokenEstimatorV1 {
		return fmt.Errorf("%w: renderer or token estimator is not registered", ErrInvalidProjection)
	}
	if projection.RenderedBytes != len([]byte(projection.Rendered)) ||
		projection.RenderedSHA256 != digestString(projection.Rendered) ||
		projection.EstimatedTokens != (projection.RenderedBytes+3)/4 {
		return fmt.Errorf("%w: rendered evidence is inconsistent", ErrInvalidProjection)
	}
	if len(projection.Selected) == 0 || projection.Selected == nil || projection.Omitted == nil ||
		len(projection.Selected) > maxSpecItems || len(projection.Selected)+len(projection.Omitted) > maxProjectionRecords {
		return fmt.Errorf("%w: selected and omitted records must be explicit arrays", ErrInvalidProjection)
	}
	seen := make(map[string]struct{}, len(projection.Selected)+len(projection.Omitted))
	seenRefs := make(map[string]string, len(projection.Selected)+len(projection.Omitted))
	for _, selected := range projection.Selected {
		if err := taskstate.ValidateRef(selected.Ref); err != nil || roleRank(selected.Role) == 0 ||
			authorityRank(selected.Authority) == 0 || selected.RenderedBytes < 1 ||
			!validDigest(selected.ContentSHA256) || selected.SourceFreshness != SourceFreshnessValidatedCurrent ||
			validateSourceRefs(selected.SourceRefs) != nil {
			return fmt.Errorf("%w: selected item %q is invalid", ErrInvalidProjection, selected.ItemID)
		}
		if err := requireExact(string(selected.ItemID), "selected item ID", ErrInvalidProjection); err != nil {
			return err
		}
		if _, duplicate := seen[string(selected.ItemID)]; duplicate {
			return fmt.Errorf("%w: item %q appears more than once", ErrInvalidProjection, selected.ItemID)
		}
		seen[string(selected.ItemID)] = struct{}{}
		if err := addProjectionRef(seenRefs, selected.Ref); err != nil {
			return err
		}
	}
	for _, omitted := range projection.Omitted {
		if err := taskstate.ValidateRef(omitted.Ref); err != nil || roleRank(omitted.Role) == 0 ||
			!validOmissionReason(omitted.Reason) || !validOmissionSource(omitted) {
			return fmt.Errorf("%w: omitted item %q is invalid", ErrInvalidProjection, omitted.ItemID)
		}
		if err := requireExact(string(omitted.ItemID), "omitted item ID", ErrInvalidProjection); err != nil {
			return err
		}
		if omitted.Reason != OmittedRoleNotSelected && omitted.SelectorID == "" {
			return fmt.Errorf("%w: omission %q requires its selector", ErrInvalidProjection, omitted.ItemID)
		}
		if omitted.Reason == OmittedRoleNotSelected && omitted.SelectorID != "" {
			return fmt.Errorf("%w: unselected role omission %q cannot name a selector", ErrInvalidProjection, omitted.ItemID)
		}
		if omitted.SelectorID != "" {
			if err := requireExact(omitted.SelectorID, "omission selector ID", ErrInvalidProjection); err != nil {
				return err
			}
		}
		if _, duplicate := seen[string(omitted.ItemID)]; duplicate {
			return fmt.Errorf("%w: item %q appears more than once", ErrInvalidProjection, omitted.ItemID)
		}
		seen[string(omitted.ItemID)] = struct{}{}
		if err := addProjectionRef(seenRefs, omitted.Ref); err != nil {
			return err
		}
	}
	if err := validateRenderedProjection(projection); err != nil {
		return err
	}
	expected, err := projectionID(projection)
	if err != nil || expected != projection.ID {
		return fmt.Errorf("%w: identity does not match exact projection", ErrInvalidProjection)
	}
	return nil
}

func addProjectionRef(seen map[string]string, ref taskstate.Ref) error {
	identity := taskstate.RefIdentity(ref)
	if hash, exists := seen[identity]; exists {
		if hash != ref.Hash {
			return fmt.Errorf("%w: reference identity has conflicting hashes", ErrInvalidProjection)
		}
		return fmt.Errorf("%w: reference identity appears more than once", ErrInvalidProjection)
	}
	seen[identity] = ref.Hash
	return nil
}

func validateRenderedProjection(projection Projection) error {
	decoder := json.NewDecoder(strings.NewReader(projection.Rendered))
	decoder.DisallowUnknownFields()
	var envelope renderedEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return fmt.Errorf("%w: decode rendered context: %v", ErrInvalidProjection, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("%w: rendered context has trailing content", ErrInvalidProjection)
	}
	if envelope.Schema != RendererJSONV1 || envelope.SpecName != projection.SpecName ||
		envelope.SpecVersion != projection.SpecVersion || envelope.ScopeRef != projection.ScopeRef ||
		len(envelope.Items) != len(projection.Selected) {
		return fmt.Errorf("%w: rendered context header or item count is inconsistent", ErrInvalidProjection)
	}
	for index, item := range envelope.Items {
		selected := projection.Selected[index]
		if item.Ref != selected.Ref || item.Role != string(selected.Role) ||
			item.Authority != string(selected.Authority) ||
			digestString(item.Content) != selected.ContentSHA256 ||
			len([]byte(item.Content)) != selected.RenderedBytes {
			return fmt.Errorf("%w: rendered item %d disagrees with selected evidence", ErrInvalidProjection, index)
		}
	}
	canonical, err := json.Marshal(envelope)
	if err != nil || string(canonical) != projection.Rendered {
		return fmt.Errorf("%w: rendered context is not canonical JSON", ErrInvalidProjection)
	}
	return nil
}

func projectionID(projection Projection) (string, error) {
	projection.ID = ""
	raw, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode context projection identity: %w", err)
	}
	return "context_projection_" + digestString(string(raw)), nil
}

func digestJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode context specification: %w", err)
	}
	return digestString(string(raw)), nil
}

func digestString(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}

func validProjectionID(value string) bool {
	return strings.HasPrefix(value, "context_projection_") && validDigest(strings.TrimPrefix(value, "context_projection_"))
}

func validOmissionReason(reason OmissionReason) bool {
	switch reason {
	case OmittedRoleNotSelected, OmittedMissingMaterial, OmittedAuthority,
		OmittedSelectorLimit, OmittedItemBudget:
		return true
	default:
		return false
	}
}
