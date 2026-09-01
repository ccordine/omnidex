package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	directCodingBrowserVerificationTextValue   = "omnidex verification input"
	directCodingBrowserVerificationNumberValue = "7"
)

// renderDirectCodingBrowserVerificationDeclaration owns every structural byte
// of browser verification. The frozen interaction surface is deterministic
// parser output and never crosses a model boundary.
func renderDirectCodingBrowserVerificationDeclaration(
	ref assemblyline.SourceBlockRef,
	binding directCodingBrowserPublicSurfaceBinding,
) (string, error) {
	if ref.Block.Role != assemblyline.SourceBlockTaskVerification ||
		ref.Block.ID != binding.verificationBlockID {
		return "", fmt.Errorf(
			"code-owned browser verification requires its exact frozen verification block",
		)
	}
	switch binding.resultRelation.Relation {
	case assemblyline.ApplicationRequirementNoDerivedResult:
	case assemblyline.ApplicationRequirementExplicitResultRelation:
		return "", fmt.Errorf(
			"browser verification %s requires an exact derived-result oracle that deterministic code does not possess",
			ref.Block.ID,
		)
	case assemblyline.ApplicationRequirementMissingResultRelation:
		return "", fmt.Errorf(
			"browser verification %s has a non-retainable missing result relation",
			ref.Block.ID,
		)
	default:
		return "", fmt.Errorf(
			"browser verification %s has unregistered result relation %q",
			ref.Block.ID, binding.resultRelation.Relation,
		)
	}

	statements, err := renderDirectCodingBrowserMechanicalVerificationStatements(
		binding.surface,
	)
	if err != nil {
		return "", fmt.Errorf("browser verification %s: %w", ref.Block.ID, err)
	}
	source := strings.TrimSpace(ref.Block.Signature) + " {\n  " +
		strings.Join(statements, "\n  ") + "\n}"
	if _, err := assemblyline.ParseTypeScriptFunction(
		assemblyline.TypeScriptFunctionContract{
			Signature: ref.Block.Signature,
			TSX:       binding.verificationTSX,
			Policy:    ref.Block.Policy,
		},
		source,
	); err != nil {
		return "", fmt.Errorf("parse code-owned browser verification %s: %w", ref.Block.ID, err)
	}
	if err := validateDirectCodingBrowserAcceptanceRoleQueries(
		source,
		binding.verificationTSX,
		binding.surface,
		binding.resultRelation.Relation,
	); err != nil {
		return "", fmt.Errorf("validate code-owned browser verification %s: %w", ref.Block.ID, err)
	}
	return source, nil
}

func renderDirectCodingBrowserMechanicalVerificationStatements(
	surface directCodingBrowserPublicInteractionSurface,
) ([]string, error) {
	if _, err := renderDirectCodingBrowserPublicInteractionSurface(surface); err != nil {
		return nil, fmt.Errorf("invalid frozen public interaction surface: %w", err)
	}
	if len(surface.Controls) == 0 && len(surface.Outputs) == 0 {
		return nil, fmt.Errorf("no mechanically observable public interaction exists")
	}
	statements := make([]string, 0, len(surface.Controls)*2+len(surface.Outputs))
	for _, control := range surface.Controls {
		statements = append(
			statements,
			"expect("+directCodingBrowserControlQuery(surface.Controls, control)+").toBeInTheDocument();",
		)
	}
	for _, output := range surface.Outputs {
		statements = append(
			statements,
			"expect("+directCodingBrowserOutputQuery(output)+").toBeInTheDocument();",
		)
	}
	for _, control := range surface.Controls {
		query := directCodingBrowserControlQuery(surface.Controls, control)
		switch control.ValueKind {
		case "action", "boolean":
			statements = append(statements, "fireEvent.click("+query+");")
		case "selection":
			if control.Role == "radio" {
				statements = append(statements, "fireEvent.click("+query+");")
				continue
			}
			return nil, fmt.Errorf(
				"control role %s requires an exact selectable value that the frozen surface does not prove",
				control.Role,
			)
		case "number":
			statements = append(statements, fmt.Sprintf(
				"fireEvent.change(%s, { target: { value: %s } });",
				query, directCodingBrowserVerificationNumberValue,
			))
		case "text":
			statements = append(statements, fmt.Sprintf(
				"fireEvent.change(%s, { target: { value: %s } });",
				query, strconv.Quote(directCodingBrowserVerificationTextValue),
			))
		default:
			return nil, fmt.Errorf(
				"control role %s has unsupported value kind %q",
				control.Role, control.ValueKind,
			)
		}
	}
	return statements, nil
}

func directCodingBrowserControlQuery(
	controls []directCodingBrowserPublicControl,
	control directCodingBrowserPublicControl,
) string {
	if control.AccessibleName != "" &&
		directCodingBrowserNamedControlCount(controls, control.Role, control.AccessibleName) == 1 {
		return fmt.Sprintf(
			"screen.getByRole(%s, { name: %s })",
			strconv.Quote(control.Role), strconv.Quote(control.AccessibleName),
		)
	}
	if control.RoleCount == 1 {
		return "screen.getByRole(" + strconv.Quote(control.Role) + ")"
	}
	return fmt.Sprintf(
		"screen.getAllByRole(%s)[%d]",
		strconv.Quote(control.Role), control.RoleOrdinal-1,
	)
}

func directCodingBrowserNamedControlCount(
	controls []directCodingBrowserPublicControl,
	role string,
	name string,
) int {
	count := 0
	for _, control := range controls {
		if control.Role == role && control.AccessibleName == name {
			count++
		}
	}
	return count
}

func directCodingBrowserOutputQuery(output directCodingBrowserPublicOutput) string {
	return fmt.Sprintf(
		"screen.getByRole(%s, { name: %s })",
		strconv.Quote("status"), strconv.Quote(output.AccessibleName),
	)
}
