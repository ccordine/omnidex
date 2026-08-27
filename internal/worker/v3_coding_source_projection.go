package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

func projectDirectCodingGoFragment(
	raw string,
) (assemblyline.PortableResultProjection, error) {
	projection, err := gofragment.ProjectFunctionModelResponseProjection(raw)
	if err != nil {
		return assemblyline.PortableResultProjection{}, err
	}
	return assemblyline.NewSourceDeclarationPortableResultProjection(
		raw, projection.Source, projection.StartByte, projection.EndByte,
	)
}

func projectDirectCodingSourceDeclaration(
	language string,
	raw string,
) (assemblyline.PortableResultProjection, error) {
	switch language {
	case "go":
		return projectDirectCodingGoFragment(raw)
	case "javascript":
		return assemblyline.ProjectJavaScriptFragment(raw)
	case "java":
		return assemblyline.ProjectJavaFragment(raw)
	case "rust":
		return assemblyline.ProjectRustFragment(raw)
	case "php":
		return assemblyline.ProjectPHPFragment(raw)
	default:
		return assemblyline.PortableResultProjection{}, fmt.Errorf(
			"source declaration projection does not support language %q", language,
		)
	}
}
