package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

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
