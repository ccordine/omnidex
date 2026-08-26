package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelcontext"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
	"github.com/gryph/omnidex/internal/scrum"
)

type directCodingRequest struct {
	Instruction         string
	AdditionalAuthority []string
	Feedback            []string
	MemoryAuthorities   []assemblyline.ObjectiveMemoryAuthority
}

type directCodingSession struct {
	runtime               *nativeRuntimeV3
	request               directCodingRequest
	root                  string
	specification         *assemblyline.ApplicationSpecification
	program               *directCodingProgram
	completion            directCodingCompletionState
	sequence              int
	protectedPaths        map[string]directCodingProtectedPath
	lastCommands          []string
	repositoryIndex       *repositoryindex.Result
	plannedFiles          int
	plannedDeletes        int
	mutationJournal       []directCodingMutationJournalEntry
	cognition             *directCodingTaskCognition
	pathProvenance        assemblyline.ArtifactIdentityProvenance
	initialPaths          map[string]directCodingInitialPath
	deploymentResolution  directCodingServiceDeploymentResolution
	deploymentDisposition assemblyline.ApplicationServiceDeploymentDisposition
	deploymentOperationID string
	deploymentReceiptSHA  string
	deployedEndpoint      directCodingObservedEndpoint
	deploymentRecovery    directCodingDeploymentRecoveryHook
}

func (s *directCodingSession) Phase(phase directCodingPhase, detail string) {
	detail = trimForBudget(strings.TrimSpace(detail), 1200)
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_phase_changed", fmt.Sprintf(
		"phase=%s detail=%s", phase, safeLine(detail, "none"),
	))
}

func (s *directCodingSession) directCodingAuthority() string {
	parts := []string{s.request.Instruction}
	parts = append(parts, s.request.AdditionalAuthority...)
	parts = append(parts, s.request.Feedback...)
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

type directCodingMutationJournalEntry struct {
	Path      string
	Operation workspaceFileOperation
}

func (r *nativeRuntimeV3) runDirectCodingAction() error {
	request, err := r.directCodingRequest()
	if err != nil {
		return err
	}
	summary, err := r.runDirectCodingSession(request)
	if err != nil {
		return err
	}
	return r.complete("coding", summary, summary)
}

func (r *nativeRuntimeV3) directCodingRequest() (directCodingRequest, error) {
	if r == nil || r.claim == nil {
		return directCodingRequest{}, fmt.Errorf("direct coding requires a claimed job")
	}
	instruction := r.claim.Job.Instruction
	additionalAuthority := make([]string, 0)
	if r.claim.Job.Pipeline == model.PipelineScrum {
		metadata, err := scrum.DecodeStoredJobMetadata(r.claim.Job.Metadata)
		if err != nil {
			return directCodingRequest{}, err
		}
		cardLines, err := scrum.ContextLinesFromMetadata(r.claim.Job.Metadata)
		if err != nil {
			return directCodingRequest{}, err
		}
		card := strings.TrimSpace(strings.Join(cardLines, "\n"))
		if card == "" {
			return directCodingRequest{}, fmt.Errorf("direct Scrum coding requires authoritative card context")
		}
		if !metadata.ChannelOrigin {
			instruction = card
		} else {
			additionalAuthority = append(additionalAuthority, card)
		}
	}
	if strings.TrimSpace(instruction) == "" {
		return directCodingRequest{}, fmt.Errorf("direct coding requires a non-empty current instruction")
	}
	return directCodingRequest{
		Instruction:         instruction,
		AdditionalAuthority: cleanOrderedStrings(additionalAuthority),
		Feedback:            collectContextValuesByKey(r.claim.Contexts, "user_feedback", "replan_feedback"),
	}, nil
}

func (r *nativeRuntimeV3) runDirectCodingSession(request directCodingRequest) (string, error) {
	if r == nil || r.svc == nil {
		return "", fmt.Errorf("direct coding runtime is unavailable")
	}
	if summary, handled, err := r.recoverDeploymentBeforeWorkspace(request); handled || err != nil {
		return summary, err
	}
	scope, err := r.svc.workspaceScopeForV3Job(r.claim.Job)
	if err != nil {
		return "", err
	}
	hasExistingImplementation, err := directCodingWorkspaceHasImplementation(scope.Root, nil)
	if err != nil {
		return "", err
	}
	var indexed *repositoryindex.Result
	if hasExistingImplementation {
		if err := r.reconcileCurrentRepositoryMutation(scope.Root); err != nil {
			return "", err
		}
		result, indexErr := r.refreshExistingRepositoryIndex(scope.Root)
		if indexErr != nil {
			return "", indexErr
		}
		indexed = &result
	}
	session := &directCodingSession{
		runtime:         r,
		request:         request,
		root:            scope.Root,
		repositoryIndex: indexed,
		protectedPaths:  map[string]directCodingProtectedPath{},
		completion: directCodingCompletionState{
			AllowExistingWorkspace: len(request.Feedback) > 0 || hasExistingImplementation,
			TestsRequired:          true,
			WrittenSource:          map[string]string{},
		},
	}
	session.deploymentRecovery = newDirectCodingDeploymentRecovery(session)
	if indexed != nil {
		paths := make([]string, len(indexed.Snapshot.Files))
		for index, file := range indexed.Snapshot.Files {
			paths[index] = file.Path
		}
		provenance, provenanceErr := modelcontext.NewArtifactIdentityProvenance(paths)
		if provenanceErr != nil {
			return "", fmt.Errorf("derive indexed artifact provenance: %w", provenanceErr)
		}
		session.pathProvenance = provenance
	}
	if indexed != nil {
		return session.runExistingRepositoryChangeWorkflow()
	}
	summary, err := runDirectCodingWorkflow(session, session.completion.AllowExistingWorkspace)
	return summary, err
}

func (s *directCodingSession) nextSequence() int {
	s.sequence++
	return s.sequence
}
