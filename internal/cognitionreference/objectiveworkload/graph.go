package objectiveworkload

import (
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
)

const workloadSchemaV1 = "objective-workload.v1"

func compilationIdentity(authority Authority) CompilationID {
	return CompilationID("C" + digestFields(workloadSchemaV1, authority.SHA256))
}

func compiledWorkloadIdentity(authority Authority, requirements []Requirement) WorkloadID {
	fields := []string{workloadSchemaV1, authority.SHA256, "fixed-root-requirement-verify-materialize.v1"}
	for _, requirement := range requirements {
		fields = append(fields,
			string(requirement.ID), requirement.SourceQuote,
			fmt.Sprintf("%d", requirement.Start), fmt.Sprintf("%d", requirement.End), requirement.SHA256,
		)
	}
	return WorkloadID("W" + digestFields(fields...))
}

func expectedObjectives(requirements []Requirement, status ObjectiveStatus) []Objective {
	rootDependencies := make([]ObjectiveID, len(requirements))
	objectives := make([]Objective, 0, 1+3*len(requirements))
	for index := range requirements {
		rootDependencies[index] = ObjectiveID(fmt.Sprintf("O%03d_requirement", index+1))
	}
	objectives = append(objectives, Objective{
		ID: "O000_root", Kind: ObjectiveRoot, DependsOn: rootDependencies,
		Acceptance: []AcceptancePredicate{AcceptanceRequirementsComplete}, Status: status,
	})
	for index, requirement := range requirements {
		prefix := fmt.Sprintf("O%03d", index+1)
		requirementID := ObjectiveID(prefix + "_requirement")
		verifyID := ObjectiveID(prefix + "_verify")
		materializeID := ObjectiveID(prefix + "_materialize")
		objectives = append(objectives,
			Objective{
				ID: requirementID, Kind: ObjectiveRequirement, Parent: "O000_root",
				DependsOn: []ObjectiveID{verifyID}, RequirementID: requirement.ID,
				Acceptance: []AcceptancePredicate{AcceptanceRequirementVerified}, Status: status,
			},
			Objective{
				ID: verifyID, Kind: ObjectiveVerify, Parent: requirementID,
				DependsOn: []ObjectiveID{materializeID}, RequirementID: requirement.ID,
				Acceptance: []AcceptancePredicate{AcceptanceArtifactVerified}, Status: status,
			},
			Objective{
				ID: materializeID, Kind: ObjectiveMaterialize, Parent: requirementID,
				DependsOn: []ObjectiveID{}, RequirementID: requirement.ID,
				Acceptance: []AcceptancePredicate{AcceptanceArtifactProduced}, Status: status,
			},
		)
	}
	return objectives
}

func validateWorkload(workload Workload, requirePending bool) error {
	if err := validateAuthority(workload.Authority); err != nil {
		return err
	}
	if workload.RootObjectiveID != "O000_root" || len(workload.Requirements) == 0 ||
		len(workload.Requirements) > maxRequirements {
		return fmt.Errorf("%w: root or requirement count is invalid", ErrInvalidGraph)
	}
	previousEnd := -1
	for index, requirement := range workload.Requirements {
		wantID := RequirementID(fmt.Sprintf("R%03d", index+1))
		if requirement.ID != wantID || requirement.SourceQuote == "" ||
			len(requirement.SourceQuote) > maxRequirementBytes || !utf8.ValidString(requirement.SourceQuote) ||
			strings.ContainsRune(requirement.SourceQuote, 0) ||
			requirement.SourceQuote != strings.TrimSpace(requirement.SourceQuote) ||
			requirement.Start < 0 || requirement.End <= requirement.Start ||
			requirement.End > len(workload.Authority.Text) || requirement.Start < previousEnd ||
			!utf8.ValidString(workload.Authority.Text[:requirement.Start]) ||
			!utf8.ValidString(workload.Authority.Text[requirement.End:]) ||
			workload.Authority.Text[requirement.Start:requirement.End] != requirement.SourceQuote ||
			strings.Index(workload.Authority.Text, requirement.SourceQuote) != requirement.Start ||
			strings.Contains(workload.Authority.Text[requirement.Start+1:], requirement.SourceQuote) ||
			requirement.SHA256 != digestBytes([]byte(requirement.SourceQuote)) {
			return fmt.Errorf("%w: requirement %d is not bound to exact authority", ErrInvalidGraph, index)
		}
		previousEnd = requirement.End
	}
	if workload.ID != compiledWorkloadIdentity(workload.Authority, workload.Requirements) {
		return fmt.Errorf("%w: workload identity is not bound to exact compiled state", ErrInvalidGraph)
	}
	if err := validateObjectiveGraph(workload.Objectives, workload.RootObjectiveID); err != nil {
		return err
	}
	expected := expectedObjectives(workload.Requirements, ObjectivePending)
	if len(workload.Objectives) != len(expected) {
		return fmt.Errorf("%w: fixed topology requires %d objectives", ErrInvalidGraph, len(expected))
	}
	for index, objective := range workload.Objectives {
		shape := cloneObjective(objective)
		if shape.Status != ObjectivePending && shape.Status != ObjectiveComplete {
			return fmt.Errorf("%w: objective %q has invalid status", ErrInvalidGraph, shape.ID)
		}
		if requirePending && shape.Status != ObjectivePending {
			return fmt.Errorf("%w: compiled objective %q must be pending", ErrInvalidGraph, shape.ID)
		}
		shape.Status = ObjectivePending
		if !reflect.DeepEqual(shape, expected[index]) {
			return fmt.Errorf("%w: objective %q violates fixed code-owned topology", ErrInvalidGraph, objective.ID)
		}
	}
	return nil
}

func preflightWorkloadBounds(workload Workload) error {
	if len(workload.Requirements) == 0 || len(workload.Requirements) > maxRequirements ||
		len(workload.Objectives) == 0 || len(workload.Objectives) > maxObjectives {
		return fmt.Errorf("%w: workload collections are outside hard bounds", ErrInvalidGraph)
	}
	if !validIdentity(string(workload.ID)) || !validIdentity(string(workload.RootObjectiveID)) {
		return fmt.Errorf("%w: workload identity fields are outside hard bounds", ErrInvalidGraph)
	}
	for index, requirement := range workload.Requirements {
		if !validIdentity(string(requirement.ID)) || len(requirement.SourceQuote) > maxRequirementBytes {
			return fmt.Errorf("%w: requirement %d fields are outside hard bounds", ErrInvalidGraph, index)
		}
	}
	for index, objective := range workload.Objectives {
		if len(objective.DependsOn) > maxRequirements || len(objective.Acceptance) > 1 {
			return fmt.Errorf("%w: objective %d collections are outside hard bounds", ErrInvalidGraph, index)
		}
		if !validIdentity(string(objective.ID)) ||
			(objective.Parent != "" && !validIdentity(string(objective.Parent))) {
			return fmt.Errorf("%w: objective %d identity fields are outside hard bounds", ErrInvalidGraph, index)
		}
		for _, dependency := range objective.DependsOn {
			if !validIdentity(string(dependency)) {
				return fmt.Errorf("%w: objective %d dependency identity is outside hard bounds", ErrInvalidGraph, index)
			}
		}
	}
	return nil
}

func validateObjectiveGraph(objectives []Objective, root ObjectiveID) error {
	if len(objectives) == 0 || len(objectives) > maxObjectives {
		return fmt.Errorf("%w: objective count is outside bounds", ErrInvalidGraph)
	}
	byID := make(map[ObjectiveID]Objective, len(objectives))
	for index, objective := range objectives {
		if !validIdentity(string(objective.ID)) {
			return fmt.Errorf("%w: objective %d has invalid identity %q", ErrInvalidGraph, index, objective.ID)
		}
		if _, duplicate := byID[objective.ID]; duplicate {
			return fmt.Errorf("%w: duplicate objective %q", ErrInvalidGraph, objective.ID)
		}
		byID[objective.ID] = objective
	}
	if _, exists := byID[root]; !exists {
		return fmt.Errorf("%w: root objective is absent", ErrInvalidGraph)
	}
	for _, objective := range objectives {
		if objective.Parent != "" {
			if _, exists := byID[objective.Parent]; !exists {
				return fmt.Errorf("%w: objective %q has dangling parent", ErrInvalidGraph, objective.ID)
			}
		}
		seen := make(map[ObjectiveID]struct{}, len(objective.DependsOn))
		for _, dependency := range objective.DependsOn {
			if _, exists := byID[dependency]; !exists {
				return fmt.Errorf("%w: objective %q has dangling dependency", ErrInvalidGraph, objective.ID)
			}
			if _, duplicate := seen[dependency]; duplicate {
				return fmt.Errorf("%w: objective %q duplicates dependency %q", ErrInvalidGraph, objective.ID, dependency)
			}
			seen[dependency] = struct{}{}
		}
	}
	visiting := make(map[ObjectiveID]bool, len(objectives))
	visited := make(map[ObjectiveID]bool, len(objectives))
	var visit func(ObjectiveID, int) error
	visit = func(id ObjectiveID, depth int) error {
		if depth > maxObjectiveDepth {
			return fmt.Errorf("%w: objective graph exceeds depth %d", ErrInvalidGraph, maxObjectiveDepth)
		}
		if visiting[id] {
			return fmt.Errorf("%w: dependency cycle at %q", ErrInvalidGraph, id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range byID[id].DependsOn {
			if err := visit(dependency, depth+1); err != nil {
				return err
			}
		}
		delete(visiting, id)
		visited[id] = true
		return nil
	}
	return visit(root, 1)
}
