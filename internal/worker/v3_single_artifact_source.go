package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
	"github.com/gryph/omnidex/internal/station"
)

func (session *directCodingSession) generateSinglePlainTextArtifact(
	blueprint assemblyline.SourceBlueprint,
	adapter directCodingArtifactAdapter,
) (string, error) {
	if err := assemblyline.ValidatePlainTextSourceBlueprint(blueprint); err != nil {
		return "", err
	}
	if len(blueprint.Documents) != 1 || len(blueprint.Documents[0].Blocks) != 1 {
		return "", fmt.Errorf("plain-text compiler must emit one document containing one source block")
	}
	document := blueprint.Documents[0]
	block := document.Blocks[0]
	if !block.Generated() || block.Signature != assemblyline.TextFragmentSignature {
		return "", fmt.Errorf("plain-text compiler emitted an invalid generated block")
	}
	if err := validatePlainTextPathBlindValue(block.Contract); err != nil {
		return "", err
	}
	modelName, err := session.workerModel(station.CodingFragment)
	if err != nil {
		return "", err
	}
	input := assemblyline.FragmentGenerationInput{
		Language:     assemblyline.TextFragmentLanguage,
		Dialect:      assemblyline.TextFragmentDialect,
		Signature:    block.Signature,
		Behavior:     block.Contract,
		Capabilities: []string{}, PermittedSymbols: []string{},
	}
	runtime := directCodingWorkerRuntime(session)
	runtime.MaxAttempts = 1
	node, err := runDirectCodingLanguageFragmentWorker(
		runtime, modelName, directCodingLanguageGenerationJob{
			Subject: block.ID, Input: input,
			Project: assemblyline.ProjectTextFragment,
			Validate: func(_ assemblyline.FragmentGenerationInput, candidate string) (string, error) {
				if err := validatePlainTextPathBlindValue(candidate); err != nil {
					return "", err
				}
				return candidate, assemblyline.ValidateTextFragment(candidate)
			},
		},
	)
	if err != nil {
		return "", err
	}
	composed, err := adapter.ComposeDocument(document, assemblyline.SourceComposition{
		Generated: map[string]string{block.ID: node}, Interfaces: map[string]string{},
	})
	if err != nil {
		return "", err
	}
	if composed.Path != document.Path {
		return "", fmt.Errorf("plain-text composer changed code-owned artifact placement")
	}
	if err := validateDirectCodingArtifactSource(adapter, document.Path, []byte(composed.Source)); err != nil {
		return "", err
	}
	return composed.Source, nil
}

// validatePlainTextPathBlindValue closes the bare-dotted-name hole that exact
// provenance alone cannot see. Any token recognized by the selected adapter is
// a file identity and is forbidden in the path-blind source envelope/result.
func validatePlainTextPathBlindValue(value string) error {
	for _, token := range modelcontext.LexicalPathTokens(value) {
		adapter, _, recognized, err := recognizeDirectCodingArtifactAdapterForPath(token.Value)
		if err != nil {
			return err
		}
		if recognized && adapter.ID == assemblyline.PlainTextAdapterID {
			return fmt.Errorf("path-blind plain-text source context contains artifact identity %q", token.Value)
		}
	}
	return nil
}
