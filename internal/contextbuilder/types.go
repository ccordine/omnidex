package contextbuilder

import (
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

const (
	ProjectionSchemaV1 = "omnidex.context-projection.v1"
	RendererJSONV1     = "omnidex.context-material-json.v1"
	TokenEstimatorV1   = "utf8-bytes-div-four.v1"
)

type Selector struct {
	ID       string          `json:"id"`
	Role     workingset.Role `json:"role"`
	MinItems int             `json:"min_items"`
	MaxItems int             `json:"max_items"`
}

type ContextSpec struct {
	Name                 string                `json:"name"`
	Version              string                `json:"version"`
	ScopeRef             taskstate.Ref         `json:"scope_ref"`
	Required             []Selector            `json:"required"`
	Optional             []Selector            `json:"optional"`
	AllowedAuthorities   []taskstate.Authority `json:"allowed_authorities"`
	MaxItems             int                   `json:"max_items"`
	MaxBytes             int                   `json:"max_bytes"`
	MaxAcquisitionRounds int                   `json:"max_acquisition_rounds"`
}

type Material struct {
	ItemID     workingset.ItemID   `json:"item_id"`
	CurrentRef taskstate.Ref       `json:"current_ref"`
	SourceRefs []taskstate.Ref     `json:"source_refs"`
	Authority  taskstate.Authority `json:"authority"`
	Content    string              `json:"content"`
	ByteCost   int                 `json:"byte_cost"`
}

type OmissionReason string

type SourceFreshness string

const (
	OmittedRoleNotSelected OmissionReason = "role_not_selected"
	OmittedMissingMaterial OmissionReason = "missing_material"
	OmittedAuthority       OmissionReason = "authority_not_allowed"
	OmittedSelectorLimit   OmissionReason = "selector_limit"
	OmittedItemBudget      OmissionReason = "projection_budget"
)

const (
	SourceFreshnessValidatedCurrent SourceFreshness = "validated_current"
	SourceFreshnessUnresolved       SourceFreshness = "unresolved"
)

type Selection struct {
	ItemID          workingset.ItemID   `json:"item_id"`
	Ref             taskstate.Ref       `json:"ref"`
	SourceRefs      []taskstate.Ref     `json:"source_refs"`
	Role            workingset.Role     `json:"role"`
	Authority       taskstate.Authority `json:"authority"`
	SourceFreshness SourceFreshness     `json:"source_freshness"`
	ContentSHA256   string              `json:"content_sha256"`
	RenderedBytes   int                 `json:"rendered_bytes"`
}

type Omission struct {
	ItemID          workingset.ItemID   `json:"item_id"`
	Ref             taskstate.Ref       `json:"ref"`
	Role            workingset.Role     `json:"role"`
	SelectorID      string              `json:"selector_id,omitempty"`
	Reason          OmissionReason      `json:"reason"`
	Authority       taskstate.Authority `json:"authority,omitempty"`
	SourceFreshness SourceFreshness     `json:"source_freshness"`
}

type Projection struct {
	Schema            string           `json:"schema"`
	ID                string           `json:"id"`
	WorkID            string           `json:"work_id"`
	SpecName          string           `json:"spec_name"`
	SpecVersion       string           `json:"spec_version"`
	SpecSHA256        string           `json:"spec_sha256"`
	RendererVersion   string           `json:"renderer_version"`
	ScopeRef          taskstate.Ref    `json:"scope_ref"`
	WorkingSetID      workingset.SetID `json:"working_set_id"`
	WorkingSetVersion uint64           `json:"working_set_version"`
	Selected          []Selection      `json:"selected"`
	Omitted           []Omission       `json:"omitted"`
	Rendered          string           `json:"rendered"`
	RenderedSHA256    string           `json:"rendered_sha256"`
	RenderedBytes     int              `json:"rendered_bytes"`
	EstimatedTokens   int              `json:"estimated_tokens"`
	TokenEstimator    string           `json:"token_estimator"`
}

type BuildInput struct {
	WorkID     string
	Spec       ContextSpec
	WorkingSet *workingset.Set
	Materials  []Material
}

type selectedMaterial struct {
	item     workingset.Item
	material Material
}
