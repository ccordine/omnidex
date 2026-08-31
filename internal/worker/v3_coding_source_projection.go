package worker

import (
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
	return assemblyline.NewExactSourceDeclarationPortableResultProjection(projection.Source)
}
