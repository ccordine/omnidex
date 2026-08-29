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
	Schema       string                  `json:"schema"`
	Surface      ApplicationSurface      `json:"surface"`
	ProductQuote string                  `json:"product_quote"`
	Tasks        []FrozenApplicationTask `json:"tasks"`
}

// FreezeApplicationWorkload deterministically projects every accepted
// requirement into exactly one task in accepted source order. There is no
// semantic workload-planning stage.
func FreezeApplicationWorkload(
	specification ApplicationSpecification,
) (FrozenApplicationWorkload, error) {
	var zero FrozenApplicationWorkload
	if err := specification.Validate(); err != nil {
		return zero, fmt.Errorf("freeze application workload: %w", err)
	}
	tasks := make([]FrozenApplicationTask, len(specification.Requirements))
	for index, requirement := range specification.Requirements {
		tasks[index] = FrozenApplicationTask{
			ID:               fmt.Sprintf("task_%03d", index+1),
			RequirementID:    requirement.ID,
			RequirementQuote: requirement.SourceQuote,
		}
	}
	digest, err := applicationWorkloadDigest(
		specification.Surface, specification.ProductQuote, tasks,
	)
	if err != nil {
		return zero, err
	}
	return FrozenApplicationWorkload{
		Schema: ApplicationWorkloadFrozenSchemaV2, SHA256: digest,
		Surface: specification.Surface, ProductQuote: specification.ProductQuote,
		Tasks: tasks,
	}, nil
}

func ValidateFrozenApplicationWorkload(workload FrozenApplicationWorkload) error {
	if workload.Schema != ApplicationWorkloadFrozenSchemaV2 {
		return fmt.Errorf(
			"frozen application workload schema must be %q",
			ApplicationWorkloadFrozenSchemaV2,
		)
	}
	decoded, err := hex.DecodeString(workload.SHA256)
	if err != nil || len(decoded) != sha256.Size ||
		workload.SHA256 != strings.ToLower(workload.SHA256) {
		return fmt.Errorf("frozen application workload hash must be 64 lowercase hexadecimal characters")
	}
	specification, err := applicationSpecificationFromFrozenWorkload(workload)
	if err != nil {
		return err
	}
	want, err := FreezeApplicationWorkload(specification)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(workload, want) {
		return fmt.Errorf("frozen application workload differs from its canonical code-owned form")
	}
	return nil
}

func ValidateFrozenApplicationWorkloadFor(
	specification ApplicationSpecification,
	workload FrozenApplicationWorkload,
) error {
	want, err := FreezeApplicationWorkload(specification)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(workload, want) {
		return fmt.Errorf("frozen application workload differs from accepted application authority")
	}
	return nil
}

func applicationSpecificationFromFrozenWorkload(
	workload FrozenApplicationWorkload,
) (ApplicationSpecification, error) {
	if workload.Schema != ApplicationWorkloadFrozenSchemaV2 {
		return ApplicationSpecification{}, fmt.Errorf(
			"frozen application workload schema must be %q",
			ApplicationWorkloadFrozenSchemaV2,
		)
	}
	requirements := make([]Requirement, len(workload.Tasks))
	for index, task := range workload.Tasks {
		wantTaskID := fmt.Sprintf("task_%03d", index+1)
		wantRequirementID := fmt.Sprintf("requirement_%03d", index+1)
		if task.ID != wantTaskID || task.RequirementID != wantRequirementID ||
			strings.TrimSpace(task.RequirementQuote) == "" {
			return ApplicationSpecification{}, fmt.Errorf(
				"frozen application task %d is not the canonical accepted-requirement projection",
				index,
			)
		}
		requirements[index] = Requirement{
			ID: task.RequirementID, SourceQuote: task.RequirementQuote,
		}
	}
	specification := ApplicationSpecification{
		Surface: workload.Surface, ProductQuote: workload.ProductQuote,
		Requirements: requirements,
	}
	if err := specification.Validate(); err != nil {
		return ApplicationSpecification{}, fmt.Errorf(
			"frozen application workload authority: %w", err,
		)
	}
	return specification, nil
}

func applicationWorkloadDigest(
	surface ApplicationSurface,
	productQuote string,
	tasks []FrozenApplicationTask,
) (string, error) {
	raw, err := exactjson.Canonical(frozenApplicationWorkloadDigest{
		Schema: ApplicationWorkloadFrozenSchemaV2, Surface: surface,
		ProductQuote: productQuote, Tasks: tasks,
	})
	if err != nil {
		return "", fmt.Errorf("canonicalize frozen application workload: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
