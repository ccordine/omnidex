package worker

import (
	"fmt"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func (s *directCodingSession) publishVerifiedTaskArtifacts(
	program, isolated *directCodingProgram,
	verifyProgress func(*directCodingProgram) error,
) error {
	if s == nil || program == nil || isolated == nil || verifyProgress == nil || s.program != program {
		return fmt.Errorf("task publication requires the active program, verified task, and progress verifier")
	}
	stage, err := projectDirectCodingAcceptedTaskStage(*program)
	if err != nil {
		return err
	}
	publication, err := directCodingTaskPublicationAssembly(*program, stage)
	if err != nil {
		return err
	}
	if len(publication.Files) == 0 || directCodingAssembliesEqual(publication, s.publishedAssembly) {
		return nil
	}
	// A newly combined source graph must pass its real compiler/tests. The
	// single-task graph has already passed those checks and is not run twice.
	isolatedAssembly, err := directCodingAssemblyFromProgram(*isolated)
	if err != nil {
		return err
	}
	combinedAssembly, err := directCodingAssemblyFromProgram(stage)
	if err != nil {
		return err
	}
	if !directCodingAssembliesEqual(isolatedAssembly, combinedAssembly) {
		if err := verifyProgress(&stage); err != nil {
			return fmt.Errorf("verify combined accepted task artifacts: %w", err)
		}
		after, err := directCodingAssemblyFromProgram(stage)
		if err != nil {
			return err
		}
		if !directCodingAssembliesEqual(combinedAssembly, after) {
			return fmt.Errorf("progress verification changed accepted source")
		}
	}
	// Validate independent absence/preservation obligations without applying
	// them as part of this task's source publication.
	obligations := publication
	obligations.RequiredPaths = program.RequiredPaths
	obligations.DeletePaths = program.DeletePaths
	if _, err := s.directCodingAssemblyDesiredStates(obligations); err != nil {
		return err
	}
	prepared, err := s.prepareWorkspaceReconciliation(publication)
	if err != nil {
		return err
	}
	if err := s.ApplyAndVerify(prepared); err != nil {
		return err
	}
	s.publishedAssembly = cloneDirectCodingAssembly(publication)
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_task_artifacts_published", fmt.Sprintf(
		"files=%d changes=%d", len(publication.Files), len(prepared.result.Changes),
	))
	return nil
}

func cloneDirectCodingAssembly(assembly directCodingAssembly) directCodingAssembly {
	clone := directCodingAssembly{
		Files:         append([]directCodingFileTask(nil), assembly.Files...),
		RequiredPaths: append([]string(nil), assembly.RequiredPaths...),
		DeletePaths:   append([]string(nil), assembly.DeletePaths...),
	}
	for index := range clone.Files {
		clone.Files[index].Content = append([]byte(nil), clone.Files[index].Content...)
	}
	return clone
}

func (s *directCodingSession) retainVerifiedPublicationChange(change workspacefacts.Change, expected directCodingAssembly) {
	if change.Kind != workspacefacts.ChangeCreate && change.Kind != workspacefacts.ChangeReplace && change.Kind != workspacefacts.ChangeMove {
		return
	}
	for _, file := range expected.Files {
		if file.Path != change.Path {
			continue
		}
		for _, retained := range s.publishedAssembly.Files {
			if retained.Path == file.Path {
				return
			}
		}
		file.Content = append([]byte(nil), file.Content...)
		s.publishedAssembly.Files = append(s.publishedAssembly.Files, file)
		return
	}
}

// Retained publication is an exact observation of this session's writes.
// Later progress may add files; it may not silently rewrite accepted bytes.
func directCodingUnpublishedAssembly(next, published directCodingAssembly) (directCodingAssembly, error) {
	retained := make(map[string]directCodingFileTask, len(published.Files))
	for _, file := range published.Files {
		retained[file.Path] = file
	}
	unpublished := directCodingAssembly{}
	for _, file := range next.Files {
		previous, exists := retained[file.Path]
		if !exists {
			unpublished.Files = append(unpublished.Files, file)
			continue
		}
		if !directCodingAssembliesEqual(
			directCodingAssembly{Files: []directCodingFileTask{previous}},
			directCodingAssembly{Files: []directCodingFileTask{file}},
		) {
			return directCodingAssembly{}, fmt.Errorf("publication changed previously accepted file %q", file.Path)
		}
		delete(retained, file.Path)
	}
	if len(retained) != 0 {
		return directCodingAssembly{}, fmt.Errorf("publication omitted %d previously accepted files", len(retained))
	}
	return unpublished, nil
}
