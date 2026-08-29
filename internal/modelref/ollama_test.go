package modelref

import "testing"

func TestValidateOllamaNameOwnsSyntaxWithoutProviderAccess(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"dolphin3:latest", "registry.example/model:Q4_K_M", "team/model@sha256"} {
		if err := ValidateOllamaName(name); err != nil {
			t.Errorf("valid name %q rejected: %v", name, err)
		}
	}
	for _, name := range []string{"", " leading", "two words", "model?tag"} {
		if err := ValidateOllamaName(name); err == nil {
			t.Errorf("invalid name %q accepted", name)
		}
	}
}
