package specialist

import "testing"

func TestDefaultTeamValidatesCohesiveSpecialistSystem(t *testing.T) {
	team := DefaultTeam()
	if err := ValidateTeam(team); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		RoleManagerSpecialist,
		RolePlannerSpecialist,
		RoleShellExecutionSpecialist,
		RoleWebResearchSpecialist,
		RoleMemorySpecialist,
		RoleCorrectionSpecialist,
		RoleExpectationSpecialist,
		RoleResearchSpecialist,
		RoleCodeSpecialist,
		RoleWorkerSpecialist,
		RoleSummarySpecialist,
	} {
		if _, ok := ProfileForRole(want); !ok {
			t.Fatalf("missing specialist profile %s in %#v", want, RoleIDs(team))
		}
	}
}

func TestShellSpecialistHasFileSystemAndResearchToolAccess(t *testing.T) {
	for _, tool := range []string{"bash", "cat", "sed", "grep", "rg", "curl", "memory.create", "research.enqueue"} {
		if !ToolAllowed(RoleShellExecutionSpecialist, tool) {
			t.Fatalf("shell specialist should allow tool %s", tool)
		}
	}
	if !MemoryActionAllowed(RoleShellExecutionSpecialist, MemoryActionCreate) {
		t.Fatal("shell specialist should be able to create evidence-backed memories")
	}
	if MemoryActionAllowed(RoleShellExecutionSpecialist, MemoryActionDeprioritize) {
		t.Fatal("shell specialist should not directly deprioritize memories; delegate correction/memory policy")
	}
}

func TestMemoryCorrectionAndManagerCanUpdateMemoryPriority(t *testing.T) {
	for _, roleID := range []string{RoleMemorySpecialist, RoleCorrectionSpecialist, RoleManagerSpecialist} {
		if !MemoryActionAllowed(roleID, MemoryActionUpdate) {
			t.Fatalf("%s should update memory", roleID)
		}
		if !MemoryActionAllowed(roleID, MemoryActionDeprioritize) {
			t.Fatalf("%s should deprioritize stale memory", roleID)
		}
	}
	if MemoryActionAllowed(RolePlannerSpecialist, MemoryActionDeprioritize) {
		t.Fatal("planner should route stale-memory changes through memory/correction specialists")
	}
}

func TestAllSpecialistsCanContributeEvidenceBackedMemory(t *testing.T) {
	for _, profile := range DefaultTeam() {
		if !profile.Memory.CanCreate {
			t.Fatalf("%s cannot create memory", profile.Role.ID)
		}
		if !profile.Memory.WritesRequireEvidence {
			t.Fatalf("%s memory writes should require evidence", profile.Role.ID)
		}
		if profile.ContextContribution == "" {
			t.Fatalf("%s missing context contribution", profile.Role.ID)
		}
	}
}
