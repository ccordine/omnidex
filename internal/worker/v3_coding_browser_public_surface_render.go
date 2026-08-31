package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	directCodingBrowserPublicSurfaceMaxSourceBytes  = 128 * 1024
	directCodingBrowserPublicSurfaceMaxNodes        = 20_000
	directCodingBrowserPublicSurfaceMaxControls     = assemblyline.MaxFragmentPublicInteractionControls
	directCodingBrowserPublicSurfaceMaxOutputs      = assemblyline.MaxFragmentPublicInteractionOutputs
	directCodingBrowserPublicSurfaceMaxLiteralBytes = assemblyline.MaxFragmentPublicInteractionLiteralBytes
)

type directCodingBrowserPublicInteractionSurface struct {
	Controls   []directCodingBrowserPublicControl
	Outputs    []directCodingBrowserPublicOutput
	ElementIDs []string
}

type directCodingBrowserPublicControl struct {
	Role            string
	RoleOrdinal     int
	RoleCount       int
	AccessibleName  string
	PlaceholderHint string
	ValueKind       string
}

type directCodingBrowserPublicOutput struct {
	AccessibleName string
}

type directCodingBrowserPendingControl struct {
	directCodingBrowserPublicControl
	id         string
	label      int
	buttonText string
}

type directCodingBrowserPendingLabel struct {
	forID    string
	literal  string
	controls []int
}

type directCodingBrowserPublicSurfaceExtractor struct {
	source      []byte
	outputFlow  directCodingBrowserOutputDataflow
	controls    []directCodingBrowserPendingControl
	labels      []directCodingBrowserPendingLabel
	outputs     []directCodingBrowserPublicOutput
	ids         []string
	seenIDs     map[string]struct{}
	seenOutputs map[string]struct{}
}

type directCodingBrowserJSXAttribute struct {
	present bool
	boolean bool
	literal string
}

func renderDirectCodingBrowserPublicInteractionSurface(
	surface directCodingBrowserPublicInteractionSurface,
) (string, error) {
	portable, err := directCodingBrowserPortablePublicInteractionSurface(surface)
	if err != nil {
		return "", err
	}
	return portable.Render()
}

func directCodingBrowserPortablePublicInteractionSurface(
	surface directCodingBrowserPublicInteractionSurface,
) (assemblyline.FragmentPublicInteractionSurface, error) {
	controls := make([]assemblyline.FragmentPublicInteractionControl, len(surface.Controls))
	for index, control := range surface.Controls {
		controls[index] = assemblyline.FragmentPublicInteractionControl{
			Role:        assemblyline.FragmentPublicInteractionRole(control.Role),
			RoleOrdinal: control.RoleOrdinal, RoleCount: control.RoleCount,
			AccessibleName: control.AccessibleName, PlaceholderHint: control.PlaceholderHint,
			ValueKind: assemblyline.FragmentPublicInteractionValueKind(control.ValueKind),
		}
	}
	outputs := make([]assemblyline.FragmentPublicOutput, len(surface.Outputs))
	for index, output := range surface.Outputs {
		outputs[index] = assemblyline.FragmentPublicOutput{
			AccessibleName: output.AccessibleName,
		}
	}
	portable := assemblyline.FragmentPublicInteractionSurface{
		Schema:   assemblyline.FragmentPublicInteractionSurfaceSchemaV1,
		Controls: controls, Outputs: outputs,
	}
	if err := portable.Validate(); err != nil {
		return assemblyline.FragmentPublicInteractionSurface{}, fmt.Errorf(
			"browser public surface is not portable: %w", err,
		)
	}
	return portable, nil
}
