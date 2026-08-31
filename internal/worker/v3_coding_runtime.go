package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrum"
)

type directCodingRequest struct {
	Instruction string
	Feedback    []string
}

type directCodingSession struct {
	runtime               *nativeRuntimeV3
	request               directCodingRequest
	root                  string
	specification         *assemblyline.ApplicationSpecification
	program               *directCodingProgram
	sequence              int
	protectedPaths        map[string]struct{}
	plannedFiles          int
	plannedDeletes        int
	mutationJournal       []directCodingMutationJournalEntry
	pathProvenance        assemblyline.ArtifactIdentityProvenance
}

func (s *directCodingSession) Phase(phase directCodingPhase, detail string) {
	detail = trimForBudget(strings.TrimSpace(detail), 1200)
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_phase_changed", fmt.Sprintf(
		"phase=%s detail=%s", phase, safeLine(detail, "none"),
	))
}

func (s *directCodingSession) directCodingAuthority() string {
	parts := []string{s.request.Instruction}
	parts = append(parts, s.request.Feedback...)
	return strings.Join(parts, "\n")
}

type directCodingMutationJournalEntry struct {
	Path      string
	Operation workspaceFileOperation
}

type workspaceFileOperation string

const (
	workspaceFileCreate  workspaceFileOperation = "create"
	workspaceFileReplace workspaceFileOperation = "replace"
	workspaceFileDelete  workspaceFileOperation = "delete"
	workspaceFileMove    workspaceFileOperation = "move"
)

func (r *nativeRuntimeV3) runDirectCodingAction() error {
	request, err := r.directCodingRequest()
	if err != nil {
		return err
	}
	summary, err := r.runDirectCodingSession(request)
	if err != nil {
		return err
	}
	return r.completeAppliedWorkspace(summary)
}

func (r *nativeRuntimeV3) directCodingRequest() (directCodingRequest, error) {
	if r == nil || r.claim == nil {
		return directCodingRequest{}, fmt.Errorf("direct coding requires a claimed job")
	}
	instruction := r.claim.Job.Instruction
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
			instruction = strings.TrimSpace(strings.Join([]string{instruction, card}, "\n"))
		}
	}
	if strings.TrimSpace(instruction) == "" {
		return directCodingRequest{}, fmt.Errorf("direct coding requires a non-empty current instruction")
	}
	request := directCodingRequest{Instruction: instruction}
	if r.claim.Job.CurrentGeneration > 1 {
		replan, err := r.svc.repo.CurrentReplanAuthority(r.ctx, r.claim.Job, "v3_coding")
		if err != nil {
			return directCodingRequest{}, err
		}
		request.Feedback = []string{replan.Feedback}
	}
	return request, nil
}

func (r *nativeRuntimeV3) runDirectCodingSession(request directCodingRequest) (string, error) {
	if r == nil || r.svc == nil {
		return "", fmt.Errorf("direct coding runtime is unavailable")
	}
	scope, err := r.svc.workspaceScopeForV3Job(r.claim.Job)
	if err != nil {
		return "", err
	}
	session := &directCodingSession{
		runtime:        r,
		request:        request,
		root:           scope.Root,
		protectedPaths: map[string]struct{}{},
	}
	summary, err := runDirectCodingWorkflow(session)
	return summary, err
}

func (s *directCodingSession) nextSequence() int {
	s.sequence++
	return s.sequence
}
