package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingBrowserPublicSurfaceBinding struct {
	taskID                string
	implementationBlockID string
	verificationBlockID   string
	verificationTSX       bool
	surface               directCodingBrowserPublicInteractionSurface
	portable              assemblyline.FragmentPublicInteractionSurface
	receipt               string
	resultRelation        assemblyline.ApplicationRequirementCandidateResultRelationResult
}

func validateDirectCodingBrowserPublicInteractionCandidate(source string) error {
	return validateDirectCodingBrowserPublicInteractionCandidateWithRuntimeCalls(source, nil)
}

func validateDirectCodingBrowserPublicInteractionCandidateWithRuntimeCalls(
	source string,
	permittedRuntimeCalls []string,
) error {
	surface, err := extractDirectCodingBrowserPublicInteractionSurfaceWithRuntimeCalls(
		source, permittedRuntimeCalls,
	)
	if err != nil {
		return err
	}
	portable, err := directCodingBrowserPortablePublicInteractionSurface(surface)
	if err != nil {
		return err
	}
	if _, err := portable.Render(); err != nil {
		return fmt.Errorf("render browser public interaction candidate: %w", err)
	}
	return nil
}

func deriveDirectCodingBrowserPublicSurfaceBinding(
	stage *directCodingProgram,
	taskID string,
) (directCodingBrowserPublicSurfaceBinding, error) {
	if stage == nil || strings.TrimSpace(taskID) == "" {
		return directCodingBrowserPublicSurfaceBinding{}, fmt.Errorf(
			"browser public-surface binding requires one stage and task",
		)
	}
	implementationID, err := directCodingTaskBlockIDByRole(
		stage.Source, taskID, assemblyline.SourceBlockTaskImplementation,
	)
	if err != nil {
		return directCodingBrowserPublicSurfaceBinding{}, err
	}
	block, exists := directCodingSourceBlueprintBlock(stage.Source, implementationID)
	if !exists || block.TaskID != taskID || !block.Generated() {
		return directCodingBrowserPublicSurfaceBinding{}, fmt.Errorf(
			"browser public-surface implementation %s is not one generated task block",
			implementationID,
		)
	}
	if !directCodingTypeScriptBlockIsTSX(stage.Source, implementationID) {
		return directCodingBrowserPublicSurfaceBinding{}, fmt.Errorf(
			"browser public-surface implementation %s is not TSX", implementationID,
		)
	}
	source := strings.TrimSpace(stage.Generated[implementationID])
	if source == "" {
		return directCodingBrowserPublicSurfaceBinding{}, fmt.Errorf(
			"browser public-surface implementation %s has no accepted declaration",
			implementationID,
		)
	}
	if _, err := assemblyline.ParseTypeScriptFunction(
		assemblyline.TypeScriptFunctionContract{
			Signature: block.Signature, TSX: true, Policy: block.Policy,
		},
		source,
	); err != nil {
		return directCodingBrowserPublicSurfaceBinding{}, fmt.Errorf(
			"revalidate browser public-surface implementation %s: %w",
			implementationID, err,
		)
	}
	permittedRuntimeCalls, err := directCodingBrowserHostCallsForBlock(block)
	if err != nil {
		return directCodingBrowserPublicSurfaceBinding{}, err
	}
	surface, err := extractDirectCodingBrowserPublicInteractionSurfaceWithRuntimeCalls(
		source, permittedRuntimeCalls,
	)
	if err != nil {
		return directCodingBrowserPublicSurfaceBinding{}, fmt.Errorf(
			"extract browser public surface from implementation %s: %w",
			implementationID, err,
		)
	}
	portable, err := directCodingBrowserPortablePublicInteractionSurface(surface)
	if err != nil {
		return directCodingBrowserPublicSurfaceBinding{}, err
	}
	receipt, err := portable.Render()
	if err != nil {
		return directCodingBrowserPublicSurfaceBinding{}, fmt.Errorf(
			"render browser public surface for implementation %s: %w",
			implementationID, err,
		)
	}
	return directCodingBrowserPublicSurfaceBinding{
		taskID: taskID, implementationBlockID: implementationID,
		surface: surface, portable: portable, receipt: receipt,
	}, nil
}

func (executor *directCodingTypeScriptProjectStageExecutor) bindBrowserPublicSurface(
	context assemblyline.ApplicationTaskContext,
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (*assemblyline.FragmentPublicInteractionSurface, func(string) error, error) {
	taskID := context.Task.TaskID
	if ref.Block.Role != assemblyline.SourceBlockTaskVerification ||
		ref.Block.TaskID != taskID {
		return nil, nil, fmt.Errorf(
			"browser public surface can bind only the current task verification declaration",
		)
	}
	if executor.publicSurfaceBindings == nil {
		return nil, nil, fmt.Errorf("browser public-surface binding authority is unavailable")
	}
	if _, exists := executor.publicSurfaceBindings[taskID]; exists {
		return nil, nil, fmt.Errorf("browser public surface is already bound for task %s", taskID)
	}
	resultRelation, err := stage.RequirementRelations.bindingForTask(stage.Workload, taskID)
	if err != nil {
		return nil, nil, err
	}
	if context.WorkloadSHA256 != stage.Workload.SHA256 ||
		context.Task.RequirementID != resultRelation.RequirementID {
		return nil, nil, fmt.Errorf(
			"browser public-surface result relation differs from current task authority",
		)
	}
	if err := resultRelation.Receipt.ValidateAcceptedFor(
		context.Task.RequirementQuote,
	); err != nil {
		return nil, nil, fmt.Errorf("browser public-surface result relation: %w", err)
	}
	binding, err := deriveDirectCodingBrowserPublicSurfaceBinding(stage, taskID)
	if err != nil {
		return nil, nil, err
	}
	if !directCodingBrowserBlockDirectlyDependsOn(ref.Block, binding.implementationBlockID) {
		return nil, nil, fmt.Errorf(
			"browser verification %s does not directly depend on implementation %s",
			ref.Block.ID, binding.implementationBlockID,
		)
	}
	binding.verificationBlockID = ref.Block.ID
	binding.verificationTSX = directCodingTypeScriptDocumentIsTSX(ref.Document)
	binding.resultRelation = resultRelation.Receipt
	executor.publicSurfaceBindings[taskID] = binding
	portable := cloneFragmentPublicInteractionSurface(binding.portable)
	validate := func(candidate string) error {
		return validateDirectCodingBrowserAcceptanceRoleQueries(
			candidate, binding.verificationTSX, binding.surface,
			binding.resultRelation.Relation,
		)
	}
	return &portable, validate, nil
}

func (executor *directCodingTypeScriptProjectStageExecutor) validateTaskBrowserPublicSurface(
	stage *directCodingProgram,
	taskID string,
) error {
	binding, exists := executor.publicSurfaceBindings[taskID]
	if !exists {
		return fmt.Errorf("browser task %s has no frozen public-surface binding", taskID)
	}
	current, err := deriveDirectCodingBrowserPublicSurfaceBinding(stage, taskID)
	if err != nil {
		return err
	}
	currentResultRelation, err := stage.RequirementRelations.bindingForTask(
		stage.Workload, taskID,
	)
	if err != nil {
		return err
	}
	verificationID, err := directCodingTaskBlockIDByRole(
		stage.Source, taskID, assemblyline.SourceBlockTaskVerification,
	)
	if err != nil {
		return err
	}
	if current.implementationBlockID != binding.implementationBlockID ||
		verificationID != binding.verificationBlockID || current.receipt != binding.receipt ||
		currentResultRelation.Receipt != binding.resultRelation ||
		!directCodingBrowserSamePublicIDs(current.surface.ElementIDs, binding.surface.ElementIDs) {
		return fmt.Errorf(
			"browser task %s public interaction surface drifted after verification binding",
			taskID,
		)
	}
	return nil
}

func (executor *directCodingTypeScriptProjectStageExecutor) validateAllBrowserPublicSurfaces(
	program *directCodingProgram,
) error {
	if program == nil || len(executor.publicSurfaceBindings) != len(program.Workload.Tasks) {
		return fmt.Errorf("complete browser program lacks one public-surface binding per task")
	}
	if err := program.RequirementRelations.validateCompleteFor(program.Workload); err != nil {
		return err
	}
	for _, task := range program.Workload.Tasks {
		if err := executor.validateTaskBrowserPublicSurface(program, task.ID); err != nil {
			return err
		}
	}
	return validateDirectCodingBrowserPublicElementIDs(
		executor.publicSurfaceBindings, program.Workload.Tasks,
	)
}

func validateDirectCodingBrowserPublicElementIDs(
	bindings map[string]directCodingBrowserPublicSurfaceBinding,
	tasks []assemblyline.FrozenApplicationTask,
) error {
	const staticMountOwner = "code-owned browser mount"
	publicIDs := map[string]string{"root": staticMountOwner}
	for _, task := range tasks {
		binding, exists := bindings[task.ID]
		if !exists {
			return fmt.Errorf("browser task %s has no frozen public-surface binding", task.ID)
		}
		for _, id := range binding.surface.ElementIDs {
			if owner, duplicate := publicIDs[id]; duplicate {
				if owner == staticMountOwner {
					return fmt.Errorf(
						"browser task %s repeats code-owned browser mount element id %q",
						task.ID, id,
					)
				}
				return fmt.Errorf(
					"browser tasks %s and %s repeat public element id %q",
					owner, task.ID, id,
				)
			}
			publicIDs[id] = task.ID
		}
	}
	return nil
}

func directCodingBrowserSamePublicIDs(first []string, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func directCodingBrowserBlockDirectlyDependsOn(
	block assemblyline.SourceBlock,
	dependencyID string,
) bool {
	for _, candidate := range block.DependsOn {
		if candidate == dependencyID {
			return true
		}
	}
	return false
}

func cloneFragmentPublicInteractionSurface(
	surface assemblyline.FragmentPublicInteractionSurface,
) assemblyline.FragmentPublicInteractionSurface {
	return assemblyline.FragmentPublicInteractionSurface{
		Schema: surface.Schema,
		Controls: append(
			[]assemblyline.FragmentPublicInteractionControl(nil), surface.Controls...,
		),
		Outputs: append([]assemblyline.FragmentPublicOutput(nil), surface.Outputs...),
	}
}
