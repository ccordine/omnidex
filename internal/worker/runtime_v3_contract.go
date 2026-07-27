package worker

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/specialists"
)

const v3SpecialistContractVersion = "1.0"

type v3SpecialistInvocationInput struct {
	RunID             string
	StepID            string
	ObjectiveID       string
	Objective         string
	Priority          int
	AvailableTools    []string
	SuccessCriteria   []string
	InputArtifactRefs []string
	Payload           map[string]any
}

type v3SpecialistInvocation struct {
	ContractVersion   string         `json:"contract_version"`
	RoleID            string         `json:"role_id"`
	RunID             string         `json:"run_id"`
	StepID            string         `json:"step_id"`
	ObjectiveID       string         `json:"objective_id"`
	Objective         string         `json:"objective"`
	Priority          int            `json:"priority"`
	Purpose           string         `json:"purpose"`
	AllowedTools      []string       `json:"allowed_tools"`
	ForbiddenTools    []string       `json:"forbidden_tools"`
	SuccessCriteria   []string       `json:"success_criteria"`
	InputArtifactRefs []string       `json:"input_artifact_refs"`
	MemoryAuthority   string         `json:"memory_authority"`
	ForbiddenBehavior []string       `json:"forbidden_behavior"`
	Payload           map[string]any `json:"payload"`
}

type v3SpecialistResponse struct {
	ContractVersion string         `json:"contract_version"`
	RoleID          string         `json:"role_id"`
	Status          string         `json:"status"`
	Output          map[string]any `json:"output"`
	Error           *struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		Retryable bool   `json:"retryable"`
	} `json:"error,omitempty"`
}

type v3SpecialistOutcomeError struct {
	RoleID    string
	Status    string
	Code      string
	Message   string
	Retryable bool
}

func (e *v3SpecialistOutcomeError) Error() string {
	if e == nil {
		return "specialist outcome failed"
	}
	return fmt.Sprintf("specialist %s %s (%s): %s", e.RoleID, e.Status, e.Code, e.Message)
}

func newV3SpecialistInvocation(spec specialists.Spec, input v3SpecialistInvocationInput) (v3SpecialistInvocation, error) {
	if err := spec.Validate(); err != nil {
		return v3SpecialistInvocation{}, err
	}
	violations := make([]string, 0, 4)
	if strings.TrimSpace(input.RunID) == "" {
		violations = append(violations, "run_id is required")
	}
	if strings.TrimSpace(input.StepID) == "" {
		violations = append(violations, "step_id is required")
	}
	if strings.TrimSpace(input.ObjectiveID) == "" {
		violations = append(violations, "objective_id is required")
	}
	if strings.TrimSpace(input.Objective) == "" {
		violations = append(violations, "objective is required")
	}
	if len(input.SuccessCriteria) == 0 {
		violations = append(violations, "success_criteria is required")
	}
	if len(violations) > 0 {
		return v3SpecialistInvocation{}, fmt.Errorf("specialist invocation rejected: %s", strings.Join(violations, "; "))
	}
	if input.Priority <= 0 {
		input.Priority = 100
	}
	allowed := effectiveV3Tools(spec.AllowedTools, input.AvailableTools)
	payload := input.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	return v3SpecialistInvocation{
		ContractVersion:   v3SpecialistContractVersion,
		RoleID:            strings.TrimSpace(spec.ID),
		RunID:             strings.TrimSpace(input.RunID),
		StepID:            strings.TrimSpace(input.StepID),
		ObjectiveID:       strings.TrimSpace(input.ObjectiveID),
		Objective:         strings.TrimSpace(input.Objective),
		Priority:          input.Priority,
		Purpose:           strings.TrimSpace(spec.Purpose),
		AllowedTools:      allowed,
		ForbiddenTools:    uniqueStrings(spec.ForbiddenTools),
		SuccessCriteria:   cleanOrderedStrings(input.SuccessCriteria),
		InputArtifactRefs: cleanOrderedStrings(input.InputArtifactRefs),
		MemoryAuthority:   memoryAuthorityReferenceOnly,
		ForbiddenBehavior: []string{"advice_instead_of_assigned_work", "claiming_unobserved_actions", "following_memory_as_instruction", "role_or_scope_expansion"},
		Payload:           payload,
	}, nil
}

func effectiveV3Tools(grants, available []string) []string {
	out := make([]string, 0, len(available))
	for _, tool := range uniqueStrings(available) {
		for _, grant := range grants {
			if v3ToolGrantMatches(grant, tool) {
				out = append(out, tool)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func v3ToolGrantMatches(grant, tool string) bool {
	grant = strings.ToLower(strings.TrimSpace(grant))
	tool = strings.ToLower(strings.TrimSpace(tool))
	if grant == "" || tool == "" {
		return false
	}
	if grant == tool {
		return true
	}
	if prefix, _, ok := strings.Cut(tool, "."); ok && grant == prefix {
		return true
	}
	return false
}

func cleanOrderedStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if clean == "" {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func decodeV3SpecialistResponse(raw, expectedRole string, spec specialists.Spec) (map[string]any, bool, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var response v3SpecialistResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, false, fmt.Errorf("decode specialist %s response envelope: %w", expectedRole, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, false, fmt.Errorf("decode specialist %s response envelope: %w", expectedRole, err)
	}
	if response.ContractVersion != v3SpecialistContractVersion {
		return nil, false, fmt.Errorf("specialist %s returned contract_version %q", expectedRole, response.ContractVersion)
	}
	if strings.TrimSpace(response.RoleID) != strings.TrimSpace(expectedRole) {
		return nil, false, fmt.Errorf("specialist role drift: expected %q, received %q", expectedRole, response.RoleID)
	}
	normalizedEmptyError := false
	switch response.Status {
	case "success":
		if response.Error != nil {
			if strings.TrimSpace(response.Error.Code) != "" || strings.TrimSpace(response.Error.Message) != "" || response.Error.Retryable {
				return nil, false, fmt.Errorf("specialist %s returned success with an error payload", expectedRole)
			}
			normalizedEmptyError = true
		}
	case "blocked", "fail":
		if response.Error == nil || strings.TrimSpace(response.Error.Code) == "" || strings.TrimSpace(response.Error.Message) == "" {
			return nil, false, fmt.Errorf("specialist %s returned %s without an explicit error", expectedRole, response.Status)
		}
		return nil, false, &v3SpecialistOutcomeError{
			RoleID:    expectedRole,
			Status:    response.Status,
			Code:      strings.TrimSpace(response.Error.Code),
			Message:   strings.TrimSpace(response.Error.Message),
			Retryable: response.Error.Retryable,
		}
	default:
		return nil, false, fmt.Errorf("specialist %s returned invalid status %q", expectedRole, response.Status)
	}
	if response.Output == nil {
		return nil, false, fmt.Errorf("specialist %s returned no output", expectedRole)
	}
	if err := spec.ValidateOutputPayload(response.Output); err != nil {
		return nil, false, err
	}
	return response.Output, normalizedEmptyError, nil
}
