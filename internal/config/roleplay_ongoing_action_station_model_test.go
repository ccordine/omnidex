package config

import "testing"

func TestLoadRoleplaySemanticModelFromExactEnvironmentKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("OMNI_ROLEPLAY_SEMANTIC_MODEL", "roleplay-model")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RoleplaySemanticModel != "roleplay-model" {
		t.Fatalf("roleplay semantic model=%q", cfg.RoleplaySemanticModel)
	}
}
