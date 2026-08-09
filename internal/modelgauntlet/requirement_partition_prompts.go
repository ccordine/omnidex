package modelgauntlet

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type requirementPartitionBriefing struct {
	Lens assemblyline.RequirementPartitionLens
}

func (requirementPartitionBriefing) advisoryBriefing() {}

type requirementPartitionAdvisoryStation struct {
	input assemblyline.RequirementPartitionInput
}

func (requirementPartitionAdvisoryStation) workKind() assemblyline.WorkKind {
	return assemblyline.WorkRequirementPartition
}

func (station requirementPartitionAdvisoryStation) buildBriefingPrompt(authoritativePrompt string) (string, error) {
	if err := station.validateAuthoritativePrompt(authoritativePrompt); err != nil {
		return "", err
	}
	job, err := assemblyline.NewRequirementPartitionBriefingJob(station.input)
	if err != nil {
		return "", err
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	return prompt, err
}

func (requirementPartitionAdvisoryStation) briefingResponseSchema() map[string]any {
	return assemblyline.RequirementPartitionBriefingResponseSchema()
}

func (requirementPartitionAdvisoryStation) decodeBriefing(raw string) (advisoryBriefing, error) {
	decision, err := assemblyline.DecodeRequirementPartitionBriefing(raw)
	if err != nil {
		return nil, err
	}
	return requirementPartitionBriefing{Lens: decision.Lens}, nil
}

func (station requirementPartitionAdvisoryStation) buildDeliberationPrompt(
	authoritativePrompt string,
	briefing advisoryBriefing,
) (string, error) {
	if err := station.validateAuthoritativePrompt(authoritativePrompt); err != nil {
		return "", err
	}
	decision, okay := briefing.(requirementPartitionBriefing)
	if !okay {
		return "", fmt.Errorf("requirement partition briefing has type %T, expected requirementPartitionBriefing", briefing)
	}
	job, err := assemblyline.NewRequirementPartitionAdvisoryJob(assemblyline.RequirementPartitionAdvisoryInput{
		Original: station.input, Lens: decision.Lens,
	})
	if err != nil {
		return "", err
	}
	prompt, schema, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return "", err
	}
	if schema != nil {
		return "", fmt.Errorf("requirement partition advisory renderer unexpectedly returned a structured schema")
	}
	return prompt, nil
}

func (requirementPartitionAdvisoryStation) synthesisInstruction() string {
	return "Return the requirement partition under the original exact-quote contract below."
}

func (station requirementPartitionAdvisoryStation) buildSynthesisPrompt(
	authoritativePrompt string,
	memo rawDeliberation,
) (string, error) {
	if err := station.validateAuthoritativePrompt(authoritativePrompt); err != nil {
		return "", err
	}
	if strings.TrimSpace(memo.Content) == "" {
		return "", fmt.Errorf("requirement partition synthesis requires final advisory content")
	}
	job, err := assemblyline.NewRequirementPartitionSynthesisJob(assemblyline.RequirementPartitionSynthesisInput{
		Original: station.input, AdvisoryMemo: memo.Content,
	})
	if err != nil {
		return "", err
	}
	prompt, schema, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return "", err
	}
	if len(schema) == 0 {
		return "", fmt.Errorf("requirement partition synthesis renderer returned no structured schema")
	}
	return prompt, nil
}

func (station requirementPartitionAdvisoryStation) validateCandidate(raw string) error {
	_, err := decodeRequirementPartition(raw, station.input)
	return err
}

func (station requirementPartitionAdvisoryStation) validateAuthoritativePrompt(candidate string) error {
	job, err := assemblyline.NewRequirementPartitionJob(station.input)
	if err != nil {
		return err
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return err
	}
	if candidate != prompt {
		return fmt.Errorf("requirement partition authoritative prompt differs from its immutable job renderer")
	}
	return nil
}
