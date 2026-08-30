package assemblyline

import (
	"fmt"
	"strings"
)

const (
	ApplicationContextSchemaV1              = "omnidex.application-context.v1"
	MaxApplicationContextFacts              = 12
	MaxApplicationContextFactBytes          = 1024
	MaxApplicationContextQuestionCandidates = 3
	maxApplicationEvidenceQuestionBytes     = 512
)

type ApplicationWorkspaceState string

const (
	ApplicationWorkspaceEmpty    ApplicationWorkspaceState = "empty"
	ApplicationWorkspaceExisting ApplicationWorkspaceState = "existing"
)

type ApplicationContextFactKind string

const (
	ApplicationContextWorkspaceState ApplicationContextFactKind = "workspace_state"
	ApplicationContextRepositoryFact ApplicationContextFactKind = "repository_fact"
	ApplicationContextExternalFact   ApplicationContextFactKind = "external_fact"
	ApplicationContextRuntimeFact    ApplicationContextFactKind = "runtime_fact"
)

type ApplicationContextAuthority string

const (
	ApplicationContextCodeAuthority     ApplicationContextAuthority = "code"
	ApplicationContextEvidenceAuthority ApplicationContextAuthority = "verified_evidence"
)

type ApplicationContextFact struct {
	ID           string                      `json:"id"`
	Kind         ApplicationContextFactKind  `json:"kind"`
	Authority    ApplicationContextAuthority `json:"authority"`
	NeedID       string                      `json:"need_id,omitempty"`
	Value        string                      `json:"value"`
	SourceID     string                      `json:"source_id"`
	SourceSHA256 string                      `json:"source_sha256"`
}

type ApplicationContext struct {
	Schema         string                    `json:"schema"`
	WorkspaceState ApplicationWorkspaceState `json:"workspace_state"`
	RequestSHA256  string                    `json:"request_sha256"`
	Facts          []ApplicationContextFact  `json:"facts"`
}

func validateApplicationRequest(label, request string) error {
	if request == "" || request != strings.TrimSpace(request) {
		return fmt.Errorf("%s require one trimmed user request", label)
	}
	if len(request) > maxPortablePayloadBytes/2 {
		return fmt.Errorf("%s user request exceeds %d bytes", label, maxPortablePayloadBytes/2)
	}
	return nil
}

func BootstrapApplicationContext(
	request string,
	workspace ApplicationWorkspaceState,
) (ApplicationContext, error) {
	var zero ApplicationContext
	if err := validateApplicationRequest("application context bootstrap", request); err != nil {
		return zero, err
	}
	if err := validateApplicationWorkspaceState(workspace); err != nil {
		return zero, err
	}
	facts := []ApplicationContextFact{{
		ID: "fact_001", Kind: ApplicationContextWorkspaceState,
		Authority: ApplicationContextCodeAuthority, Value: string(workspace),
		SourceID: "workspace", SourceSHA256: ExactObjectiveContextSHA(string(workspace)),
	}}
	context := ApplicationContext{
		Schema: ApplicationContextSchemaV1, WorkspaceState: workspace,
		RequestSHA256: ExactObjectiveContextSHA(request), Facts: facts,
	}
	if err := context.Validate(); err != nil {
		return zero, err
	}
	return context, nil
}

func (context ApplicationContext) Validate() error {
	if context.Schema != ApplicationContextSchemaV1 {
		return fmt.Errorf("application context schema must be %q", ApplicationContextSchemaV1)
	}
	if err := validateApplicationWorkspaceState(context.WorkspaceState); err != nil {
		return err
	}
	if len(context.Facts) < 1 || len(context.Facts) > MaxApplicationContextFacts {
		return fmt.Errorf("application context requires between 1 and %d facts", MaxApplicationContextFacts)
	}
	seen := make(map[string]struct{}, len(context.Facts))
	for index, fact := range context.Facts {
		if fact.ID != fmt.Sprintf("fact_%03d", index+1) {
			return fmt.Errorf("application context fact %d has non-code-owned identity %q", index, fact.ID)
		}
		if _, duplicate := seen[fact.ID]; duplicate {
			return fmt.Errorf("application context fact %q is duplicated", fact.ID)
		}
		seen[fact.ID] = struct{}{}
		if fact.Value == "" || fact.Value != strings.TrimSpace(fact.Value) ||
			len(fact.Value) > MaxApplicationContextFactBytes {
			return fmt.Errorf("application context fact %q has invalid value", fact.ID)
		}
		if err := ValidatePathFreeModelContext("application context fact "+fact.ID, fact.Value); err != nil {
			return err
		}
		if fact.SourceID == "" || fact.SourceID != strings.TrimSpace(fact.SourceID) {
			return fmt.Errorf("application context fact %q requires one source identity", fact.ID)
		}
		if fact.SourceSHA256 != ExactObjectiveContextSHA(fact.Value) {
			return fmt.Errorf("application context fact %q source hash does not match", fact.ID)
		}
		if err := validateApplicationContextFactBoundary(fact); err != nil {
			return fmt.Errorf("application context fact %q: %w", fact.ID, err)
		}
	}
	workspace := context.Facts[0]
	if workspace.Kind != ApplicationContextWorkspaceState ||
		workspace.Authority != ApplicationContextCodeAuthority ||
		workspace.Value != string(context.WorkspaceState) {
		return fmt.Errorf("application context first fact must be the code-owned workspace state")
	}
	return nil
}

func validateApplicationWorkspaceState(state ApplicationWorkspaceState) error {
	switch state {
	case ApplicationWorkspaceEmpty, ApplicationWorkspaceExisting:
		return nil
	default:
		return fmt.Errorf("application workspace state %q is unsupported", state)
	}
}

func validateApplicationContextFactBoundary(fact ApplicationContextFact) error {
	switch fact.Kind {
	case ApplicationContextWorkspaceState:
		if fact.Authority != ApplicationContextCodeAuthority {
			return fmt.Errorf("workspace state requires code authority")
		}
		if fact.NeedID != "" {
			return fmt.Errorf("workspace state cannot cite an evidence need")
		}
	case ApplicationContextRepositoryFact, ApplicationContextExternalFact, ApplicationContextRuntimeFact:
		if fact.Authority != ApplicationContextEvidenceAuthority {
			return fmt.Errorf("acquired fact requires verified evidence authority")
		}
		if fact.NeedID == "" || fact.NeedID != strings.TrimSpace(fact.NeedID) {
			return fmt.Errorf("acquired fact requires one evidence-need identity")
		}
	default:
		return fmt.Errorf("kind %q is unsupported", fact.Kind)
	}
	return nil
}
