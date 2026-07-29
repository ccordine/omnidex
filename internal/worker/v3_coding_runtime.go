package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/scrum"
	"github.com/gryph/omnidex/internal/specialists"
)

type directCodingRequest struct {
	Instruction         string
	AdditionalAuthority []string
	Feedback            []string
}

type directCodingSession struct {
	runtime         *nativeRuntimeV3
	request         directCodingRequest
	root            string
	specification   *assemblyline.ApplicationSpecification
	program         *directCodingProgram
	completion      directCodingCompletionState
	sequence        int
	protectedPaths  map[string]directCodingProtectedPath
	skillCandidates []specialists.SkillVersion
	lastCommands    []string
	plannedFiles    int
	plannedDeletes  int
}

func requiresDirectCoding(objective artifacts.Objective) bool {
	return objective.RequiresAction &&
		containsString(objective.RequiredCapabilities, capabilityWorkspaceWrite) &&
		containsString(objective.RequiredCapabilities, capabilityCommandExecute)
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

func (r *nativeRuntimeV3) runDirectCodingObjective(
	_ v3SubtaskAssignment,
	objective artifacts.Objective,
	_ []string,
) (string, []string, error) {
	if !requiresDirectCoding(objective) {
		return "", nil, fmt.Errorf("objective %q does not authorize direct coding", objective.ID)
	}
	request, err := r.directCodingRequest()
	if err != nil {
		return "", nil, err
	}
	summary, err := r.runDirectCodingSession(request)
	if err != nil {
		return "", nil, err
	}
	return summary, []string{"workspace"}, nil
}

func (r *nativeRuntimeV3) directCodingRequest() (directCodingRequest, error) {
	if r == nil || r.claim == nil {
		return directCodingRequest{}, fmt.Errorf("direct coding requires a claimed job")
	}
	instruction := strings.TrimSpace(r.claim.Job.Instruction)
	additionalAuthority := make([]string, 0)
	if scrum.IsScrumJob(r.claim.Job.Metadata) {
		cardLines := scrum.ContextLinesFromMetadata(r.claim.Job.Metadata)
		card := strings.TrimSpace(strings.Join(cardLines, "\n"))
		if card == "" {
			return directCodingRequest{}, fmt.Errorf("direct Scrum coding requires authoritative card context")
		}
		metadata, err := strictV3MetadataObject(r.claim.Job.Metadata)
		if err != nil {
			return directCodingRequest{}, err
		}
		channelOrigin, present, err := strictMetadataBool(metadata, "scrum_channel_origin")
		if err != nil {
			return directCodingRequest{}, err
		}
		if !present || !channelOrigin {
			instruction = card
		} else {
			additionalAuthority = append(additionalAuthority, card)
		}
	}
	if instruction == "" {
		return directCodingRequest{}, fmt.Errorf("direct coding requires a non-empty current instruction")
	}
	return directCodingRequest{
		Instruction:         instruction,
		AdditionalAuthority: cleanOrderedStrings(additionalAuthority),
		Feedback:            collectContextValuesByKey(r.claim.Contexts, "user_feedback", "replan_feedback"),
	}, nil
}

func (r *nativeRuntimeV3) runDirectCodingSession(request directCodingRequest) (string, error) {
	if r == nil || r.svc == nil || r.svc.v3Tools == nil {
		return "", fmt.Errorf("direct coding runtime is unavailable")
	}
	scope, err := r.svc.workspaceScopeForV3Job(r.claim.Job)
	if err != nil {
		return "", err
	}
	hasExistingImplementation, err := directCodingWorkspaceHasImplementation(scope.Root, nil)
	if err != nil {
		return "", err
	}
	session := &directCodingSession{
		runtime:        r,
		request:        request,
		root:           scope.Root,
		protectedPaths: map[string]directCodingProtectedPath{},
		completion: directCodingCompletionState{
			AllowExistingWorkspace: len(request.Feedback) > 0 || hasExistingImplementation,
			TestsRequired:          true,
			WrittenSource:          map[string]string{},
		},
	}
	summary, err := runDirectCodingWorkflow(session, session.completion.AllowExistingWorkspace)
	if err == nil {
		return summary, nil
	}
	if rejectErr := session.rejectPendingSkills(err); rejectErr != nil {
		return "", fmt.Errorf("%w; %v", err, rejectErr)
	}
	return "", err
}

func (s *directCodingSession) nextSequence() int {
	s.sequence++
	return s.sequence
}
