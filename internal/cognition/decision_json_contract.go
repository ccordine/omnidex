package cognition

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func validateDecisionJSONContract(raw []byte) error {
	root, err := decodeDecisionObject(raw, "decision")
	if err != nil {
		return err
	}
	if err := requireDecisionFields(root,
		[]string{"obligation_id", "action", "evidence_refs", "expected_effect"},
		[]string{"ledger_proposals", "attention_requests"}, "decision",
	); err != nil {
		return err
	}
	action, err := decodeDecisionObject(root["action"], "decision.action")
	if err != nil {
		return err
	}
	if err := requireDecisionFields(action, []string{"kind", "arguments"}, nil, "decision.action"); err != nil {
		return err
	}
	arguments, err := decodeDecisionArray(action["arguments"], "decision.action.arguments")
	if err != nil {
		return err
	}
	for index, argument := range arguments {
		object, err := decodeDecisionObject(argument, fmt.Sprintf("decision.action.arguments[%d]", index))
		if err != nil {
			return err
		}
		if err := requireDecisionFields(object, []string{"name", "value"}, nil,
			fmt.Sprintf("decision.action.arguments[%d]", index)); err != nil {
			return err
		}
	}
	if _, err := decodeDecisionArray(root["evidence_refs"], "decision.evidence_refs"); err != nil {
		return err
	}
	if proposals, exists := root["ledger_proposals"]; exists {
		values, err := decodeDecisionArray(proposals, "decision.ledger_proposals")
		if err != nil {
			return err
		}
		for index, proposal := range values {
			if err := validateProposalJSON(proposal, index); err != nil {
				return err
			}
		}
	}
	if attention, exists := root["attention_requests"]; exists {
		values, err := decodeDecisionArray(attention, "decision.attention_requests")
		if err != nil {
			return err
		}
		for index, request := range values {
			path := fmt.Sprintf("decision.attention_requests[%d]", index)
			object, err := decodeDecisionObject(request, path)
			if err != nil {
				return err
			}
			if err := requireDecisionFields(
				object, []string{"operation", "target_ref", "scope", "reason"}, nil, path,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateProposalJSON(raw json.RawMessage, index int) error {
	path := fmt.Sprintf("decision.ledger_proposals[%d]", index)
	object, err := decodeDecisionObject(raw, path)
	if err != nil {
		return err
	}
	kindRaw, exists := object["kind"]
	if !exists {
		return fmt.Errorf("%s is missing required field %q", path, "kind")
	}
	var kind LedgerProposalKind
	if err := json.Unmarshal(kindRaw, &kind); err != nil || kind == "" {
		return fmt.Errorf("%s kind is invalid", path)
	}
	switch kind {
	case ProposalObservation, ProposalHypothesis:
		if err := requireDecisionFields(object, []string{"kind", "content", "evidence_refs"}, nil, path); err != nil {
			return err
		}
		_, err = decodeDecisionArray(object["evidence_refs"], path+".evidence_refs")
		return err
	case ProposalQuestion:
		if err := requireDecisionFields(object, []string{"kind", "content"}, []string{"evidence_refs"}, path); err != nil {
			return err
		}
		if evidence, present := object["evidence_refs"]; present {
			_, err = decodeDecisionArray(evidence, path+".evidence_refs")
		}
		return err
	case ProposalObligation:
		return validateTypedProposalJSON(object, path, "obligation", "desired")
	case ProposalRevision:
		return validateTypedProposalJSON(object, path, "revision", "target_ref")
	case ProposalPlanRevision:
		return validateTypedProposalJSON(object, path, "plan_revision", "next")
	default:
		return nil
	}
}

func validateTypedProposalJSON(
	object map[string]json.RawMessage,
	path string,
	field string,
	valueField string,
) error {
	if err := requireDecisionFields(object, []string{"kind", field}, nil, path); err != nil {
		return err
	}
	payloadPath := path + "." + field
	payload, err := decodeDecisionObject(object[field], payloadPath)
	if err != nil {
		return err
	}
	if err := requireDecisionFields(payload, []string{valueField, "evidence_refs"}, nil, payloadPath); err != nil {
		return err
	}
	if _, err := decodeDecisionArray(payload["evidence_refs"], payloadPath+".evidence_refs"); err != nil {
		return err
	}
	if valueField == "desired" || valueField == "next" {
		return validateGoalJSON(payload[valueField], payloadPath+"."+valueField)
	}
	return nil
}

func validateGoalJSON(raw json.RawMessage, path string) error {
	goal, err := decodeDecisionObject(raw, path)
	if err != nil {
		return err
	}
	if err := requireDecisionFields(goal, nil, []string{"all", "any", "not"}, path); err != nil {
		return err
	}
	for _, field := range []string{"all", "any", "not"} {
		if predicates, exists := goal[field]; exists {
			if _, err := decodeDecisionArray(predicates, path+"."+field); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeDecisionObject(raw []byte, path string) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return nil, fmt.Errorf("%s must be an explicit JSON object", path)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	return object, nil
}

func decodeDecisionArray(raw []byte, path string) ([]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '[' {
		return nil, fmt.Errorf("%s must be an explicit JSON array", path)
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	return values, nil
}

func requireDecisionFields(
	object map[string]json.RawMessage,
	required []string,
	optional []string,
	path string,
) error {
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, field := range required {
		allowed[field] = struct{}{}
		if _, exists := object[field]; !exists {
			return fmt.Errorf("%s is missing required field %q", path, field)
		}
	}
	for _, field := range optional {
		allowed[field] = struct{}{}
	}
	for field := range object {
		if _, exists := allowed[field]; !exists {
			return fmt.Errorf("%s contains field %q outside its tagged contract", path, field)
		}
	}
	return nil
}
