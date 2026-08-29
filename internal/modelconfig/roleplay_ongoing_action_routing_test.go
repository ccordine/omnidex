package modelconfig

import "testing"

func TestApplyRoleplaySemanticRouting(t *testing.T) {
	applied := Apply(Routing{}, Config{
		"roleplay_semantic_model": "roleplay-model",
	})
	if got := applied.RoleplaySemanticModel; got != "roleplay-model" {
		t.Fatalf("roleplay semantic model=%q", got)
	}
}
