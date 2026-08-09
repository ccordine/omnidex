package modelgauntlet

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type repositoryRetrievalBriefing struct {
	Lens assemblyline.RepositoryRetrievalLens
}

func (repositoryRetrievalBriefing) advisoryBriefing() {}

type repositoryRetrievalAdvisoryStation struct {
	input assemblyline.RepositoryRetrievalInput
}

func (repositoryRetrievalAdvisoryStation) workKind() assemblyline.WorkKind {
	return assemblyline.WorkRepositoryRetrieval
}

func (station repositoryRetrievalAdvisoryStation) buildBriefingPrompt(authoritative string) (string, error) {
	if err := station.validateAuthoritativePrompt(authoritative); err != nil {
		return "", err
	}
	job, err := assemblyline.NewRepositoryRetrievalBriefingJob(station.input)
	if err != nil {
		return "", err
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	return prompt, err
}

func (repositoryRetrievalAdvisoryStation) briefingResponseSchema() map[string]any {
	return assemblyline.RepositoryRetrievalBriefingResponseSchema()
}

func (repositoryRetrievalAdvisoryStation) decodeBriefing(raw string) (advisoryBriefing, error) {
	decision, err := assemblyline.DecodeRepositoryRetrievalBriefing(raw)
	if err != nil {
		return nil, err
	}
	return repositoryRetrievalBriefing{Lens: decision.Lens}, nil
}

func (station repositoryRetrievalAdvisoryStation) buildDeliberationPrompt(
	authoritative string,
	briefing advisoryBriefing,
) (string, error) {
	if err := station.validateAuthoritativePrompt(authoritative); err != nil {
		return "", err
	}
	decision, ok := briefing.(repositoryRetrievalBriefing)
	if !ok {
		return "", fmt.Errorf("repository retrieval briefing has type %T", briefing)
	}
	job, err := assemblyline.NewRepositoryRetrievalAdvisoryJob(assemblyline.RepositoryRetrievalAdvisoryInput{
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
		return "", fmt.Errorf("repository retrieval advisory unexpectedly has a schema")
	}
	return prompt, nil
}

func (repositoryRetrievalAdvisoryStation) synthesisInstruction() string {
	return "Return the typed repository retrieval decision under the original contract."
}

func (station repositoryRetrievalAdvisoryStation) buildSynthesisPrompt(
	authoritative string,
	memo rawDeliberation,
) (string, error) {
	if err := station.validateAuthoritativePrompt(authoritative); err != nil {
		return "", err
	}
	if strings.TrimSpace(memo.Content) == "" {
		return "", fmt.Errorf("repository retrieval synthesis requires final advisory content")
	}
	job, err := assemblyline.NewRepositoryRetrievalSynthesisJob(assemblyline.RepositoryRetrievalSynthesisInput{
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
		return "", fmt.Errorf("repository retrieval synthesis has no schema")
	}
	return prompt, nil
}

func (station repositoryRetrievalAdvisoryStation) validateCandidate(raw string) error {
	_, err := decodeRepositoryRetrieval(raw, station.input)
	return err
}

func (station repositoryRetrievalAdvisoryStation) validateAuthoritativePrompt(candidate string) error {
	job, err := assemblyline.NewRepositoryRetrievalJob(station.input)
	if err != nil {
		return err
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return err
	}
	if candidate != prompt {
		return fmt.Errorf("repository retrieval authoritative prompt differs from immutable renderer")
	}
	return nil
}
