package assemblyline

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	FragmentPublicInteractionSurfaceSchemaV1 = "omnidex.fragment-public-interaction-surface.v1"

	MaxFragmentPublicInteractionControls     = 32
	MaxFragmentPublicInteractionOutputs      = 64
	MaxFragmentPublicInteractionLiteralBytes = 256
	MaxFragmentPublicInteractionReceiptBytes = 16 * 1024
)

type FragmentPublicInteractionRole string

const (
	FragmentPublicRoleButton     FragmentPublicInteractionRole = "button"
	FragmentPublicRoleCheckbox   FragmentPublicInteractionRole = "checkbox"
	FragmentPublicRoleCombobox   FragmentPublicInteractionRole = "combobox"
	FragmentPublicRoleListbox    FragmentPublicInteractionRole = "listbox"
	FragmentPublicRoleRadio      FragmentPublicInteractionRole = "radio"
	FragmentPublicRoleSearchbox  FragmentPublicInteractionRole = "searchbox"
	FragmentPublicRoleSlider     FragmentPublicInteractionRole = "slider"
	FragmentPublicRoleSpinbutton FragmentPublicInteractionRole = "spinbutton"
	FragmentPublicRoleTextbox    FragmentPublicInteractionRole = "textbox"
)

type FragmentPublicInteractionValueKind string

const (
	FragmentPublicValueAction    FragmentPublicInteractionValueKind = "action"
	FragmentPublicValueBoolean   FragmentPublicInteractionValueKind = "boolean"
	FragmentPublicValueNumber    FragmentPublicInteractionValueKind = "number"
	FragmentPublicValueSelection FragmentPublicInteractionValueKind = "selection"
	FragmentPublicValueText      FragmentPublicInteractionValueKind = "text"
)

// FragmentPublicInteractionSurface is a path-blind, code-proven projection of
// one fragment's public interaction facts. An accessible action name is a
// public claim to verify, never proof of its behavior. The projection excludes
// source, handlers, expressions, artifact identities, and expected results.
type FragmentPublicInteractionSurface struct {
	Schema   string                             `json:"schema"`
	Controls []FragmentPublicInteractionControl `json:"controls"`
	Outputs  []FragmentPublicOutput             `json:"outputs"`
}

type FragmentPublicInteractionControl struct {
	Role            FragmentPublicInteractionRole      `json:"role"`
	RoleOrdinal     int                                `json:"role_ordinal"`
	RoleCount       int                                `json:"role_count"`
	AccessibleName  string                             `json:"accessible_name,omitempty"`
	PlaceholderHint string                             `json:"placeholder_hint,omitempty"`
	ValueKind       FragmentPublicInteractionValueKind `json:"value_kind"`
}

type FragmentPublicOutput struct {
	AccessibleName string `json:"accessible_name"`
}

func (surface FragmentPublicInteractionSurface) Validate() error {
	if surface.Schema != FragmentPublicInteractionSurfaceSchemaV1 {
		return fmt.Errorf(
			"fragment public interaction surface schema must be %q",
			FragmentPublicInteractionSurfaceSchemaV1,
		)
	}
	if len(surface.Controls) > MaxFragmentPublicInteractionControls {
		return fmt.Errorf(
			"fragment public interaction surface exceeds %d controls",
			MaxFragmentPublicInteractionControls,
		)
	}
	if len(surface.Outputs) > MaxFragmentPublicInteractionOutputs {
		return fmt.Errorf(
			"fragment public interaction surface exceeds %d outputs",
			MaxFragmentPublicInteractionOutputs,
		)
	}
	roleCounts := make(map[FragmentPublicInteractionRole]int)
	for index, control := range surface.Controls {
		if !validFragmentPublicRoleValueKind(control.Role, control.ValueKind) {
			return fmt.Errorf(
				"fragment public interaction control %d has invalid role/value-kind semantics",
				index+1,
			)
		}
		roleCounts[control.Role]++
	}
	roleOrdinals := make(map[FragmentPublicInteractionRole]int)
	for index, control := range surface.Controls {
		roleOrdinals[control.Role]++
		if control.RoleOrdinal != roleOrdinals[control.Role] ||
			control.RoleCount != roleCounts[control.Role] {
			return fmt.Errorf(
				"fragment public interaction control %d has non-canonical role ordinals",
				index+1,
			)
		}
		if err := validateFragmentPublicLiteral(control.AccessibleName); err != nil {
			return fmt.Errorf(
				"fragment public interaction control %d accessible name: %w", index+1, err,
			)
		}
		if err := validateFragmentPublicLiteral(control.PlaceholderHint); err != nil {
			return fmt.Errorf(
				"fragment public interaction control %d placeholder hint: %w", index+1, err,
			)
		}
	}
	outputNames := make(map[string]struct{}, len(surface.Outputs))
	for index, output := range surface.Outputs {
		if output.AccessibleName == "" {
			return fmt.Errorf(
				"fragment public interaction output %d requires an accessible name",
				index+1,
			)
		}
		if err := validateFragmentPublicLiteral(output.AccessibleName); err != nil {
			return fmt.Errorf(
				"fragment public interaction output %d accessible name: %w",
				index+1, err,
			)
		}
		if _, duplicate := outputNames[output.AccessibleName]; duplicate {
			return fmt.Errorf(
				"fragment public interaction outputs repeat accessible name %q",
				output.AccessibleName,
			)
		}
		outputNames[output.AccessibleName] = struct{}{}
	}
	if len(renderValidatedFragmentPublicInteractionSurface(surface)) >
		MaxFragmentPublicInteractionReceiptBytes {
		return fmt.Errorf(
			"fragment public interaction receipt exceeds %d bytes",
			MaxFragmentPublicInteractionReceiptBytes,
		)
	}
	return nil
}

// Render emits the sole canonical text receipt for a validated surface. Array
// order is public document order. Output names are exact status locators, not
// expected-result authority.
func (surface FragmentPublicInteractionSurface) Render() (string, error) {
	if err := surface.Validate(); err != nil {
		return "", err
	}
	return renderValidatedFragmentPublicInteractionSurface(surface), nil
}

func renderValidatedFragmentPublicInteractionSurface(
	surface FragmentPublicInteractionSurface,
) string {
	var receipt strings.Builder
	receipt.WriteString("PUBLIC_INTERACTION_SURFACE_V1\n")
	for index, control := range surface.Controls {
		fmt.Fprintf(
			&receipt,
			"CONTROL %d role=%s role_ordinal=%d role_count=%d accessible_name=%s placeholder_hint=%s value_kind=%s\n",
			index+1, control.Role, control.RoleOrdinal, control.RoleCount,
			fragmentPublicOptionalLiteral(control.AccessibleName),
			fragmentPublicOptionalLiteral(control.PlaceholderHint), control.ValueKind,
		)
	}
	for index, output := range surface.Outputs {
		fmt.Fprintf(
			&receipt,
			"OUTPUT %d role=status accessible_name=%s\n",
			index+1, fragmentPublicOptionalLiteral(output.AccessibleName),
		)
	}
	receipt.WriteString("END_PUBLIC_INTERACTION_SURFACE")
	return receipt.String()
}

func validFragmentPublicRoleValueKind(
	role FragmentPublicInteractionRole,
	kind FragmentPublicInteractionValueKind,
) bool {
	switch role {
	case FragmentPublicRoleButton:
		return kind == FragmentPublicValueAction
	case FragmentPublicRoleCheckbox:
		return kind == FragmentPublicValueBoolean
	case FragmentPublicRoleCombobox, FragmentPublicRoleListbox, FragmentPublicRoleRadio:
		return kind == FragmentPublicValueSelection
	case FragmentPublicRoleSlider, FragmentPublicRoleSpinbutton:
		return kind == FragmentPublicValueNumber
	case FragmentPublicRoleSearchbox, FragmentPublicRoleTextbox:
		return kind == FragmentPublicValueText
	default:
		return false
	}
}

func validateFragmentPublicLiteral(value string) error {
	if len(value) > MaxFragmentPublicInteractionLiteralBytes {
		return fmt.Errorf("literal exceeds %d bytes", MaxFragmentPublicInteractionLiteralBytes)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("literal is not valid UTF-8")
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("literal contains a control character")
	}
	if html.UnescapeString(value) != value || strings.Join(strings.Fields(value), " ") != value {
		return fmt.Errorf("literal is not in canonical display form")
	}
	return nil
}

func fragmentPublicOptionalLiteral(value string) string {
	if value == "" {
		return "NONE"
	}
	return strconv.Quote(value)
}
