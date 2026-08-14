package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

type frozenApplicationWorkloadDigest struct {
	Schema       string                                    `json:"schema"`
	Surface      ApplicationSurface                        `json:"surface"`
	ProductQuote string                                    `json:"product_quote"`
	Requirements []applicationWorkloadRequirementAuthority `json:"requirements"`
	Tasks        []FrozenApplicationTask                   `json:"tasks"`
}

func FirstApplicationWorkloadDefect(
	input ApplicationWorkloadDraftInput,
	draft ApplicationWorkloadDraft,
) *ApplicationWorkloadDefect {
	if err := validateApplicationWorkloadDraftInput(input); err != nil {
		return &ApplicationWorkloadDefect{Detail: err.Error()}
	}
	if draft.Schema != ApplicationWorkloadDraftSchemaV1 {
		return &ApplicationWorkloadDefect{Detail: fmt.Sprintf(
			"workload schema must be %q", ApplicationWorkloadDraftSchemaV1,
		)}
	}
	if len(draft.Tasks) != len(input.Requirements) {
		return &ApplicationWorkloadDefect{Detail: fmt.Sprintf(
			"workload must cover exactly %d accepted requirements", len(input.Requirements),
		)}
	}
	indices := make(map[string]int, len(input.Requirements))
	for index, requirement := range input.Requirements {
		indices[requirement.ID] = index
	}
	for index, task := range draft.Tasks {
		taskID := fmt.Sprintf("task_%03d", index+1)
		if err := validateApplicationWorkloadIdentifier("requirement identity", task.RequirementID); err != nil {
			return &ApplicationWorkloadDefect{TaskID: taskID, Detail: err.Error()}
		}
		if task.RequirementID != input.Requirements[index].ID {
			return &ApplicationWorkloadDefect{TaskID: taskID, Detail: fmt.Sprintf(
				"requirement identity %q must be %q in accepted source order",
				task.RequirementID, input.Requirements[index].ID,
			)}
		}
		if err := validateApplicationWorkloadLine("objective", task.Objective, maxApplicationObjectiveRunes); err != nil {
			return taskDefect(taskID, ApplicationWorkloadObjectiveField, err)
		}
		if err := validateApplicationJobSpecificationList(
			"required behavior", task.RequiredBehaviors,
			maxApplicationRequiredBehaviors, maxApplicationBehaviorRunes,
		); err != nil {
			return taskDefect(taskID, ApplicationWorkloadRequiredBehaviorsField, err)
		}
		if err := validateApplicationJobSpecificationList(
			"acceptance criterion", task.AcceptanceCriteria,
			maxApplicationAcceptanceCriteria, maxApplicationCriterionRunes,
		); err != nil {
			return taskDefect(taskID, ApplicationWorkloadAcceptanceCriteriaField, err)
		}
		seenDependencies := make(map[string]struct{}, len(task.DependsOn))
		lastIndex := -1
		for dependencyIndex, dependency := range task.DependsOn {
			if err := validateApplicationWorkloadIdentifier("dependency identity", dependency); err != nil {
				return taskDefect(taskID, ApplicationWorkloadDependsOnField, err)
			}
			sourceIndex, exists := indices[dependency]
			if !exists {
				return taskDefect(taskID, ApplicationWorkloadDependsOnField, fmt.Errorf(
					"dependency %q is not an accepted requirement", dependency,
				))
			}
			if dependency == task.RequirementID {
				return taskDefect(taskID, ApplicationWorkloadDependsOnField, fmt.Errorf(
					"dependency %q is self-referential", dependency,
				))
			}
			if _, duplicate := seenDependencies[dependency]; duplicate {
				return taskDefect(taskID, ApplicationWorkloadDependsOnField, fmt.Errorf(
					"dependency %q is duplicated", dependency,
				))
			}
			if dependencyIndex > 0 && sourceIndex <= lastIndex {
				return taskDefect(taskID, ApplicationWorkloadDependsOnField, fmt.Errorf(
					"dependencies must preserve accepted requirement order",
				))
			}
			seenDependencies[dependency] = struct{}{}
			lastIndex = sourceIndex
		}
	}
	if cycleTask := firstApplicationWorkloadCycle(draft, indices); cycleTask >= 0 {
		return taskDefect(
			fmt.Sprintf("task_%03d", cycleTask+1), ApplicationWorkloadDependsOnField,
			fmt.Errorf("dependencies contain a cycle"),
		)
	}
	return nil
}

func ValidateApplicationWorkloadDraft(input ApplicationWorkloadDraftInput, draft ApplicationWorkloadDraft) error {
	if defect := FirstApplicationWorkloadDefect(input, draft); defect != nil {
		return defect
	}
	return nil
}

func FreezeApplicationWorkload(
	input ApplicationWorkloadDraftInput,
	draft ApplicationWorkloadDraft,
) (FrozenApplicationWorkload, error) {
	var zero FrozenApplicationWorkload
	if err := ValidateApplicationWorkloadDraft(input, draft); err != nil {
		return zero, fmt.Errorf("freeze application workload: %w", err)
	}
	taskIDs := make(map[string]string, len(draft.Tasks))
	for index, task := range draft.Tasks {
		taskIDs[task.RequirementID] = fmt.Sprintf("task_%03d", index+1)
	}
	tasks := make([]FrozenApplicationTask, 0, len(draft.Tasks))
	for index, task := range draft.Tasks {
		dependencies := make([]string, 0, len(task.DependsOn))
		for _, requirementID := range task.DependsOn {
			dependencies = append(dependencies, taskIDs[requirementID])
		}
		tasks = append(tasks, FrozenApplicationTask{
			ID: fmt.Sprintf("task_%03d", index+1), RequirementID: task.RequirementID,
			RequirementQuote: input.Requirements[index].SourceQuote, Objective: task.Objective,
			RequiredBehaviors:  append([]string{}, task.RequiredBehaviors...),
			AcceptanceCriteria: append([]string{}, task.AcceptanceCriteria...),
			DependsOn:          dependencies,
		})
	}
	digest, err := applicationWorkloadDigest(input, tasks)
	if err != nil {
		return zero, err
	}
	return FrozenApplicationWorkload{
		Schema: ApplicationWorkloadFrozenSchemaV1, SHA256: digest,
		Surface: input.Surface, ProductQuote: input.ProductQuote, Tasks: tasks,
	}, nil
}

func ValidateFrozenApplicationWorkload(
	input ApplicationWorkloadDraftInput,
	frozen FrozenApplicationWorkload,
) error {
	if err := validateApplicationWorkloadDraftInput(input); err != nil {
		return err
	}
	if frozen.Schema != ApplicationWorkloadFrozenSchemaV1 {
		return fmt.Errorf("frozen application workload schema must be %q", ApplicationWorkloadFrozenSchemaV1)
	}
	if frozen.Surface != input.Surface || frozen.ProductQuote != input.ProductQuote {
		return fmt.Errorf("frozen application workload differs from authoritative surface or product")
	}
	decoded, err := hex.DecodeString(frozen.SHA256)
	if err != nil || len(decoded) != sha256.Size || frozen.SHA256 != strings.ToLower(frozen.SHA256) {
		return fmt.Errorf("frozen application workload hash must be 64 lowercase hexadecimal characters")
	}
	wantHash, err := applicationWorkloadDigest(input, frozen.Tasks)
	if err != nil {
		return err
	}
	if frozen.SHA256 != wantHash {
		return fmt.Errorf("frozen application workload hash does not match its accepted authority")
	}
	draft, err := draftFromFrozenApplicationWorkload(input, frozen)
	if err != nil {
		return err
	}
	want, err := FreezeApplicationWorkload(input, draft)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(frozen, want) {
		return fmt.Errorf("frozen application workload differs from its canonical code-owned form")
	}
	return nil
}

func applicationWorkloadDigest(input ApplicationWorkloadDraftInput, tasks []FrozenApplicationTask) (string, error) {
	requirements := make([]applicationWorkloadRequirementAuthority, 0, len(input.Requirements))
	for _, requirement := range input.Requirements {
		requirements = append(requirements, applicationWorkloadRequirementAuthority{
			ID: requirement.ID, SourceQuote: requirement.SourceQuote,
		})
	}
	raw, err := exactjson.Canonical(frozenApplicationWorkloadDigest{
		Schema: ApplicationWorkloadFrozenSchemaV1, Surface: input.Surface, ProductQuote: input.ProductQuote,
		Requirements: requirements, Tasks: tasks,
	})
	if err != nil {
		return "", fmt.Errorf("canonicalize frozen application workload: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func taskDefect(taskID string, field ApplicationWorkloadTaskField, err error) *ApplicationWorkloadDefect {
	return &ApplicationWorkloadDefect{TaskID: taskID, Field: field, Detail: err.Error()}
}
