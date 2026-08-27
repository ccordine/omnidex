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
	project, err := directCodingSourceDeclarationProjector(language)
	if err != nil {
		return assemblyline.PortableResultProjection{}, err
	}
	return project(raw)
}

func directCodingSourceDeclarationProjector(
	language string,
) (directCodingLanguageFragmentProjector, error) {
	switch language {
	case "go":
		return projectDirectCodingGoFragment, nil
	case "javascript":
		return assemblyline.ProjectJavaScriptFragment, nil
	case "java":
		return assemblyline.ProjectJavaFragment, nil
	case "rust":
		return assemblyline.ProjectRustFragment, nil
	case "php":
		return assemblyline.ProjectPHPFragment, nil
	default:
		return nil, fmt.Errorf(
			"source declaration projection does not support language %q", language,
		)
	}
}
