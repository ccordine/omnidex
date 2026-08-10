package queue

import "testing"

func TestFreshMigrationRejectsBroadAndNonOwnedSchemas(t *testing.T) {
	for name, test := range map[string]struct {
		schema string
		owned  bool
	}{
		"empty":              {schema: "", owned: true},
		"public":             {schema: "public", owned: true},
		"catalog":            {schema: "pg_catalog", owned: true},
		"information schema": {schema: "information_schema", owned: true},
		"non owned":          {schema: "omnidex_runtime", owned: false},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateResettableRuntimeSchema(test.schema, test.owned); err == nil {
				t.Fatal("unsafe runtime schema was accepted for destructive migration")
			}
		})
	}
	if err := validateResettableRuntimeSchema("omnidex_runtime", true); err != nil {
		t.Fatal(err)
	}
}
