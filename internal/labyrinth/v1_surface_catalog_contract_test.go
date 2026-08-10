package labyrinth

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestV1SurfaceCatalogRejectsEveryIdentityAndParameterDrift(t *testing.T) {
	t.Parallel()
	generated, err := Generate(testGeneratorConfig(SuiteCombined, 77_101))
	if err != nil {
		t.Fatal(err)
	}
	exact := generated.PublicArtifact().World.Catalog
	if err := validateV1SurfaceCatalog(exact); err != nil {
		t.Fatalf("exact catalog rejected: %v", err)
	}
	for name, mutate := range map[string]func(*cognition.ActionCatalog){
		"catalog ID":      func(catalog *cognition.ActionCatalog) { catalog.ID = "changed.actions.v1" },
		"catalog version": func(catalog *cognition.ActionCatalog) { catalog.Version = "changed.v1" },
		"schema ID":       func(catalog *cognition.ActionCatalog) { catalog.Schemas[0].ID = "changed.action.v1" },
		"schema version":  func(catalog *cognition.ActionCatalog) { catalog.Schemas[0].Version = "changed.v1" },
		"maximum bytes": func(catalog *cognition.ActionCatalog) {
			index := schemaWithParameters(*catalog)
			catalog.Schemas[index].Parameters[0].MaxBytes--
		},
		"parameter order": func(catalog *cognition.ActionCatalog) {
			index := schemaWithMultipleParameters(*catalog)
			parameters := catalog.Schemas[index].Parameters
			parameters[0], parameters[1] = parameters[1], parameters[0]
		},
		"duplicate parameter": func(catalog *cognition.ActionCatalog) {
			index := schemaWithMultipleParameters(*catalog)
			catalog.Schemas[index].Parameters[1].Name = catalog.Schemas[index].Parameters[0].Name
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			changed := exact.Clone()
			mutate(&changed)
			if err := validateV1SurfaceCatalog(changed); err == nil {
				t.Fatal("drifted v1 surface catalog was accepted")
			}
		})
	}
}

func schemaWithParameters(catalog cognition.ActionCatalog) int {
	for index, schema := range catalog.Schemas {
		if len(schema.Parameters) > 0 {
			return index
		}
	}
	panic("test catalog has no parameters")
}

func schemaWithMultipleParameters(catalog cognition.ActionCatalog) int {
	for index, schema := range catalog.Schemas {
		if len(schema.Parameters) > 1 {
			return index
		}
	}
	panic("test catalog has no multi-parameter schema")
}
