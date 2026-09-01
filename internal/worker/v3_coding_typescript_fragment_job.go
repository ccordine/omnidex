package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func newDirectCodingTypeScriptPortableJob(
	job directCodingTypeScriptFragmentJob,
) (assemblyline.PortableJob, error) {
	input, err := directCodingTypeScriptFragmentInput(job)
	if err != nil {
		return assemblyline.PortableJob{}, err
	}
	return assemblyline.NewFragmentGenerationJob(input)
}

func directCodingTypeScriptFragmentInput(
	job directCodingTypeScriptFragmentJob,
) (assemblyline.FragmentGenerationInput, error) {
	if job.block.Role == assemblyline.SourceBlockTaskVerification {
		return assemblyline.FragmentGenerationInput{}, fmt.Errorf(
			"TypeScript verification declarations are rendered by deterministic code",
		)
	}
	hasInitialValidator := job.validateInitialCandidate != nil
	if hasInitialValidator && job.block.Role != assemblyline.SourceBlockTaskImplementation {
		return assemblyline.FragmentGenerationInput{}, fmt.Errorf(
			"TypeScript source-only candidate validation is restricted to implementation declarations",
		)
	}
	capabilities := make([]string, 0, 1)
	if available := strings.TrimSpace(job.available); available != "" {
		capabilities = append(capabilities, available)
	}
	return assemblyline.FragmentGenerationInput{
		Language: "typescript", Signature: strings.TrimSpace(job.block.Signature),
		Dialect:  strings.TrimSpace(job.dialect),
		Behavior: strings.TrimSpace(job.block.Contract), Capabilities: capabilities,
		PermittedSymbols: append([]string(nil), job.block.Globals...),
	}, nil
}
