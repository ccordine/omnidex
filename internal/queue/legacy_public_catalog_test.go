package queue

import "testing"

func TestLegacyCatalogNormalizationIgnoresSearchPathQualificationOnly(t *testing.T) {
	const want = `{"default": "gen_random_uuid()"}`
	for name, value := range map[string]string{
		"core function after explicit search path": `{"default": "pg_catalog.gen_random_uuid()"}`,
		"extension function before schema rename":  `{"default": "public.gen_random_uuid()"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := normalizeLegacyCatalogDefinition(value, legacyPublicSchema); got != want {
				t.Fatalf("normalized definition=%q want %q", got, want)
			}
		})
	}
}
