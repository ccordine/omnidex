package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func (s *directCodingSession) ensureDirectCodingAcceptanceGrounding(
	program *directCodingProgram,
) error {
	reviewModel, err := s.workerModel(station.CodingWorkload)
	if err != nil {
		return err
	}
	correctionModel, err := s.workerModel(station.CodingFragmentCorrection)
	if err != nil {
		return err
	}
	runtime := directCodingWorkerRuntime(s)
	runtime.CorrectionModel = correctionModel
	return ensureDirectCodingAcceptanceGrounding(runtime, reviewModel, correctionModel, program)
}

func ensureDirectCodingAcceptanceGrounding(
	runtime typedWorkerRuntime,
	reviewModel string,
	correctionModel string,
	program *directCodingProgram,
) error {
	if program == nil || runtime.Context == nil || runtime.Execute == nil {
		return fmt.Errorf("acceptance grounding requires a program and portable execution runtime")
	}
	if strings.TrimSpace(reviewModel) == "" || strings.TrimSpace(correctionModel) == "" {
		return fmt.Errorf("acceptance grounding requires review and correction models")
	}
	if program.AcceptanceGrounding == nil {
		program.AcceptanceGrounding = make(map[string]assemblyline.ApplicationAcceptanceGroundingReceipt)
	}
	for _, task := range program.Workload.Tasks {
		_, acceptanceID, err := applicationTaskBlockIDs(task.ID)
		if err != nil {
			return err
		}
		block, exists := directCodingTypeScriptBlueprintBlock(program.TypeScript, acceptanceID)
		if !exists {
			continue
		}
		if !block.Generated() {
			return fmt.Errorf("acceptance grounding target %s is not a generated declaration", acceptanceID)
		}
		if strings.TrimSpace(program.Generated[acceptanceID]) == "" {
			return fmt.Errorf("acceptance grounding target %s has no accepted source", acceptanceID)
		}
		context, _, recognized, err := directCodingAcceptanceTaskAuthority(*program, acceptanceID)
		if err != nil {
			return err
		}
		if !recognized {
			return fmt.Errorf("acceptance grounding target %s has no frozen task authority", acceptanceID)
		}
		if err := ensureOneDirectCodingAcceptanceGrounding(
			runtime, reviewModel, correctionModel, context, acceptanceID, program,
		); err != nil {
			return err
		}
	}
	return nil
}

func ensureOneDirectCodingAcceptanceGrounding(
	runtime typedWorkerRuntime,
	reviewModel string,
	correctionModel string,
	context assemblyline.ApplicationTaskContext,
	acceptanceID string,
	program *directCodingProgram,
) error {
	seenSources := make(map[string]struct{})
	for {
		if err := runtime.Context.Err(); err != nil {
			return fmt.Errorf("acceptance grounding stopped by context authority: %w", err)
		}
		source := strings.TrimSpace(program.Generated[acceptanceID])
		input, err := assemblyline.NewApplicationAcceptanceGroundingReviewInput(
			context, source, directCodingTypeScriptBlockIsTSX(program.TypeScript, acceptanceID),
			directCodingBrowserAcceptancePlatformAuthorities(),
		)
		if err != nil {
			return fmt.Errorf("inventory acceptance observations for %s: %w", acceptanceID, err)
		}
		if receipt, exists := program.AcceptanceGrounding[acceptanceID]; exists {
			if receipt.ValidateFor(input, source) == nil {
				return nil
			}
			delete(program.AcceptanceGrounding, acceptanceID)
		}
		if _, repeated := seenSources[input.SourceSHA256]; repeated {
			return fmt.Errorf("acceptance grounding correction repeated a prior source state for %s", acceptanceID)
		}
		seenSources[input.SourceSHA256] = struct{}{}

		review, err := runDirectCodingAcceptanceGroundingReview(
			runtime, reviewModel, acceptanceID, input,
		)
		if err != nil {
			return err
		}
		if review.Decision == assemblyline.AcceptanceGroundingAccept {
			receipt, err := assemblyline.AcceptApplicationAcceptanceGroundingReview(input, review)
			if err != nil {
				return err
			}
			program.AcceptanceGrounding[acceptanceID] = receipt
			return nil
		}
		corrected, err := correctDirectCodingAcceptanceGrounding(
			runtime, correctionModel, *program, acceptanceID, source, input, review,
		)
		if err != nil {
			return err
		}
		if strings.TrimSpace(corrected) == source {
			return fmt.Errorf("acceptance grounding correction returned unchanged source for %s", acceptanceID)
		}
		program.Generated[acceptanceID] = corrected
		delete(program.AcceptanceGrounding, acceptanceID)
	}
}

func runDirectCodingAcceptanceGroundingReview(
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	input assemblyline.ApplicationAcceptanceGroundingReviewInput,
) (assemblyline.ApplicationAcceptanceGroundingReview, error) {
	job, err := assemblyline.NewApplicationAcceptanceGroundingReviewJob(input)
	if err != nil {
		return assemblyline.ApplicationAcceptanceGroundingReview{}, err
	}
	return runProgressiveDirectCodingAcceptanceGroundingReview(
		runtime, modelName, subject+"_grounding", job, input,
	)
}
