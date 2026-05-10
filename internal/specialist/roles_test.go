package specialist

import (
	"strings"
	"testing"
)

func TestForPipelineActionReturnsSpecialistRole(t *testing.T) {
	role := ForPipelineAction("workspace_scan")
	if role.ID != "filesystem_research_specialist" {
		t.Fatalf("ForPipelineAction(workspace_scan).ID=%q", role.ID)
	}
	if !strings.Contains(strings.ToLower(role.Scope), "workspace") && !strings.Contains(strings.ToLower(role.Scope), "repository") {
		t.Fatalf("unexpected scope: %q", role.Scope)
	}
}

func TestForLocalCapabilityReturnsSpecialistRole(t *testing.T) {
	role := ForLocalCapability("local_media")
	if role.ID != "media_control_specialist" {
		t.Fatalf("ForLocalCapability(local_media).ID=%q", role.ID)
	}
	if !strings.Contains(strings.ToLower(role.Scope), "vlc") {
		t.Fatalf("expected VLC scope detail, got: %q", role.Scope)
	}
}

func TestSummaryAndDetailLines(t *testing.T) {
	role := ForPipelineAction("verify")
	summary := Summary(role)
	if !strings.Contains(summary, "review_verification_specialist") {
		t.Fatalf("summary missing specialist id: %q", summary)
	}

	lines := DetailLines(role)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"Specialist:",
		"Specialist ID:",
		"Specialist Scope:",
		"Specialist Responsibilities:",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("detail lines missing %q: %q", want, joined)
		}
	}
}

func TestEnvVarForRoleID(t *testing.T) {
	tests := []struct {
		roleID string
		want   string
	}{
		{roleID: RolePlannerSpecialist, want: "OLLAMA_MODEL_SPECIALIST_PLANNER"},
		{roleID: RoleBrowserInspectionSpecialist, want: "OLLAMA_MODEL_SPECIALIST_BROWSER_INSPECTION"},
		{roleID: RoleReviewVerificationSpecialist, want: "OLLAMA_MODEL_SPECIALIST_REVIEW_VERIFICATION"},
		{roleID: "unknown", want: ""},
	}

	for _, tc := range tests {
		got := EnvVarForRoleID(tc.roleID)
		if got != tc.want {
			t.Fatalf("EnvVarForRoleID(%q)=%q want %q", tc.roleID, got, tc.want)
		}
	}
}
