package cognitiongauntlet

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

// witnessPolicyClient is test machinery, not a cognition baseline. It follows a
// private witness solely to prove that the production runtime is actually wired
// through its durable policy, projection, action, and terminal ports.
type witnessPolicyClient struct {
	mu           sync.Mutex
	model        string
	witness      []labyrinth.WitnessAction
	evidenceUses []labyrinth.EvidenceUse
	acquired     map[cognition.ActionID][]cognition.EvidenceRef
	next         int
	prompts      []string
}

type witnessPolicyEnvelope struct {
	ActionCatalog    cognition.ActionCatalog `json:"action_catalog"`
	EvidenceRefs     []cognition.EvidenceRef `json:"evidence_refs"`
	ProjectedContext struct {
		Items []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"items"`
	} `json:"projected_context"`
}

func (client *witnessPolicyClient) Generate(
	_ context.Context,
	modelName string,
	prompt string,
) (string, error) {
	return "", fmt.Errorf("witness policy forbids unprepared generation for model %q and %d prompt bytes", modelName, len(prompt))
}

func (client *witnessPolicyClient) generateDecision(
	modelName string,
	prompt string,
) (string, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if modelName != client.model {
		return "", fmt.Errorf("witness policy received model %q", modelName)
	}
	if client.next >= len(client.witness) {
		return "", fmt.Errorf("witness policy received an extra model call")
	}
	var envelope witnessPolicyEnvelope
	if err := json.Unmarshal([]byte(prompt), &envelope); err != nil {
		return "", err
	}
	current, err := witnessCurrentObligation(envelope)
	if err != nil || envelope.ActionCatalog.Validate() != nil {
		return "", fmt.Errorf("witness policy received an invalid production envelope")
	}
	action := client.witness[client.next]
	decisionRequest := action.Request.Clone()
	schema, exists := envelope.ActionCatalog.Schema(action.Request.Kind)
	if !exists {
		shell, shellExists := envelope.ActionCatalog.Schema("shell")
		if !shellExists {
			return "", fmt.Errorf("witness action %q is absent from the model catalog", action.Request.Kind)
		}
		command, shellErr := rawShellCommand(action.Request)
		if shellErr != nil {
			return "", shellErr
		}
		decisionRequest, shellErr = cognition.NewActionRequest("shell", []cognition.ActionArgument{{
			Name: rawShellArgument, Value: command,
		}})
		if shellErr != nil {
			return "", shellErr
		}
		schema = shell
	}
	attention, err := client.captureAcquiredEvidence(envelope.EvidenceRefs)
	if err != nil {
		return "", err
	}
	evidence := make([]cognition.EvidenceRef, 0, len(attention))
	for _, request := range attention {
		evidence = appendUniqueEvidence(evidence, []cognition.EvidenceRef{request.TargetRef})
	}
	required := client.consumerEvidence(action.ID)
	if len(required) > 0 && evidenceAvailable(required, envelope.EvidenceRefs) {
		evidence = appendUniqueEvidence(evidence, required)
	}
	if schema.EvidencePolicy == cognition.EvidenceRequired {
		required = client.consumerEvidence(action.ID)
		if len(client.evidenceUses) == 0 {
			required = append(required, envelope.EvidenceRefs...)
		}
		if len(required) == 0 || !evidenceAvailable(required, envelope.EvidenceRefs) {
			return "", fmt.Errorf("required witness evidence was absent from the production snapshot")
		}
		evidence = appendUniqueEvidence(evidence, required)
	}
	decision := cognition.CognitionDecision{
		ObligationID: current.ID,
		Action:       decisionRequest, EvidenceRefs: evidence,
		ExpectedEffect: "Apply the registered transition described by the bounded action.",
		Proposals:      []cognition.LedgerProposal{}, Attention: attention,
	}
	raw, err := json.Marshal(decision)
	if err != nil {
		return "", err
	}
	client.next++
	return string(raw), nil
}

func (client *witnessPolicyClient) captureAcquiredEvidence(
	available []cognition.EvidenceRef,
) ([]cognition.AttentionRequest, error) {
	if client.next == 0 || len(client.evidenceUses) == 0 {
		return []cognition.AttentionRequest{}, nil
	}
	prior := client.witness[client.next-1].ID
	needed := make([]labyrinth.EvidenceUse, 0)
	for _, use := range client.evidenceUses {
		if use.AcquisitionActionID == prior {
			needed = append(needed, use)
		}
	}
	if len(needed) == 0 {
		return []cognition.AttentionRequest{}, nil
	}
	latest := latestEvidenceRefs(available)
	if len(latest) == 0 {
		return nil, fmt.Errorf("acquisition action %q produced no model-visible evidence", prior)
	}
	if client.acquired == nil {
		client.acquired = make(map[cognition.ActionID][]cognition.EvidenceRef)
	}
	for _, use := range needed {
		client.acquired[use.RequiredByActionID] = appendUniqueEvidence(
			client.acquired[use.RequiredByActionID], latest,
		)
	}
	return []cognition.AttentionRequest{}, nil
}

func appendUniqueEvidence(
	current []cognition.EvidenceRef,
	additional []cognition.EvidenceRef,
) []cognition.EvidenceRef {
	seen := make(map[cognition.EvidenceRef]struct{}, len(current)+len(additional))
	result := append([]cognition.EvidenceRef(nil), current...)
	for _, ref := range result {
		seen[ref] = struct{}{}
	}
	for _, ref := range additional {
		if _, exists := seen[ref]; exists {
			continue
		}
		seen[ref] = struct{}{}
		result = append(result, ref)
	}
	return result
}

func (client *witnessPolicyClient) consumerEvidence(
	actionID cognition.ActionID,
) []cognition.EvidenceRef {
	return append([]cognition.EvidenceRef(nil), client.acquired[actionID]...)
}

func latestEvidenceRefs(refs []cognition.EvidenceRef) []cognition.EvidenceRef {
	var latest uint64
	for _, ref := range refs {
		if ref.Revision.Number > latest {
			latest = ref.Revision.Number
		}
	}
	result := make([]cognition.EvidenceRef, 0)
	for _, ref := range refs {
		if latest != 0 && ref.Revision.Number == latest {
			result = append(result, ref)
		}
	}
	return result
}

func evidenceAvailable(required, available []cognition.EvidenceRef) bool {
	set := make(map[cognition.EvidenceRef]struct{}, len(available))
	for _, ref := range available {
		set[ref] = struct{}{}
	}
	for _, ref := range required {
		if _, exists := set[ref]; !exists {
			return false
		}
	}
	return true
}

func witnessCurrentObligation(envelope witnessPolicyEnvelope) (cognition.Obligation, error) {
	var matches []cognition.Obligation
	for _, item := range envelope.ProjectedContext.Items {
		if item.Role != "task" {
			continue
		}
		var obligation cognition.Obligation
		if err := json.Unmarshal([]byte(item.Content), &obligation); err != nil || obligation.ID == "" {
			return cognition.Obligation{}, fmt.Errorf("decode projected current obligation")
		}
		matches = append(matches, obligation)
	}
	if len(matches) != 1 {
		return cognition.Obligation{}, fmt.Errorf("projected current obligation matched %d items", len(matches))
	}
	return matches[0], nil
}
