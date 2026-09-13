package worker

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
	"github.com/gryph/omnidex/internal/station"
)

// generateDirectCodingGoBlock resolves one task-local Go function body. The
// model sees the code-owned declaration only as lexical scope; Go code owns
// declaration composition, placement, parsing, and acceptance validation.
func generateDirectCodingGoBlock(
	generator *directCodingLanguageSourceGenerator,
	context assemblyline.ApplicationTaskContext,
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (string, error) {
	if generator == nil || stage == nil {
		return "", fmt.Errorf("Go source generation requires one generator and isolated stage")
	}
	if generator.config.Language != "go" || generator.config.AdapterID != "go" ||
		ref.Document.AdapterID != "go" {
		return "", fmt.Errorf("Go source generator cannot build adapter %q block %s", ref.Document.AdapterID, ref.Block.ID)
	}
	if ref.Block.TaskID == "" || ref.Block.TaskID != context.Task.TaskID {
		return "", fmt.Errorf("Go source block %s differs from frozen task authority", ref.Block.ID)
	}
	if ref.Block.Role != assemblyline.SourceBlockTaskImplementation &&
		ref.Block.Role != assemblyline.SourceBlockTaskVerification &&
		ref.Block.Role != assemblyline.SourceBlockTaskExample {
		return "", fmt.Errorf("Go source generator cannot build task role %q", ref.Block.Role)
	}
	input, err := directCodingGoFragmentInput(stage, ref)
	if err != nil {
		return "", err
	}
	modelName, err := generator.session.workerModel(station.CodingFragment)
	if err != nil {
		return "", err
	}
	runtime := directCodingWorkerRuntime(generator.session)
	runtime.MaxAttempts = assemblyline.MaxSourceBodyAttempts
	return runDirectCodingLanguageFragmentWorker(runtime, modelName, directCodingLanguageGenerationJob{
		Subject: ref.Block.ID, Input: input, Validate: validateDirectCodingGoFragment,
	})
}

func directCodingGoFragmentInput(stage *directCodingProgram, ref assemblyline.SourceBlockRef) (assemblyline.FragmentGenerationInput, error) {
	input, err := directCodingLanguageFragmentInput(stage, ref, "go")
	if err != nil {
		return input, err
	}
	return input, nil
}

func validateDirectCodingGoFragment(
	input assemblyline.FragmentGenerationInput,
	candidate string,
) (string, error) {
	validated, err := validateDirectCodingGoFragmentSyntax(input, candidate)
	var defect *assemblyline.SourceBodyDefect
	if !errors.As(err, &defect) {
		return validated, err
	}
	start, end, err := defect.MutableRange(candidate)
	if err != nil {
		return "", err
	}
	replacements, err := directCodingGoIdentifierChoices(input, candidate, candidate[start:end], start)
	if err != nil {
		return "", fmt.Errorf("enumerate exact Go identifier replacements: %w", err)
	}
	if len(replacements) == 0 {
		return "", defect
	}
	bound, err := defect.WithIdentifierReplacements(replacements)
	if err != nil {
		return "", err
	}
	return "", bound
}

// Trial validation locates a defect without recursively enumerating repairs.
func validateDirectCodingGoFragmentSyntax(
	input assemblyline.FragmentGenerationInput,
	candidate string,
) (string, error) {
	permitted := append([]string(nil), input.Capabilities...)
	permitted = append(permitted, input.PermittedSymbols...)
	validated, err := gofragment.ParseNewFunctionBody(input.Signature, permitted, candidate)
	if err == nil {
		return validated, nil
	}
	var located *gofragment.BodySpanViolation
	if !errors.As(err, &located) {
		return "", err
	}
	if located.StartByte == 0 && located.EndByte == len(candidate) {
		return "", err
	}
	defect, defectErr := assemblyline.NewSourceBodyIdentifierDefect(
		candidate,
		located.StartByte,
		located.EndByte,
		"Which available value has the meaning required at this unresolved reference?",
		err,
		nil,
	)
	if defectErr != nil {
		return "", fmt.Errorf("map exact Go identifier to implementation body: %w", defectErr)
	}
	return "", defect
}

func directCodingGoFunctionName(signature string) (string, error) {
	compiled, err := gofragment.CompileNewFunctionSignature(signature)
	if err != nil {
		return "", err
	}
	return compiled.Name, nil
}
