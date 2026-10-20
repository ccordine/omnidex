package codingagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/model"
)

// Parser amendments are sparse semantic patches, not replacement domain rows.
// Packet fields (including nested acceptance/verification) are retained when
// omitted or null; only explicit fields replace the approved payload.
// Their source revision is bound by the controller, not reconstructed by AI.
type planningParserOperation struct {
	Kind        model.LedgerMutationKind `json:"kind"`
	TargetID    model.LedgerItemID       `json:"target_id,omitempty"`
	Item        json.RawMessage          `json:"item,omitempty"`
	ReviewState model.LedgerReviewState  `json:"review_state,omitempty"`
	Rank        *int64                   `json:"rank,omitempty"`
	Rationale   string                   `json:"rationale"`
}

func decodePlanningOperations(content json.RawMessage, ledger EngineeringLedger) ([]model.LedgerMutationOperation, error) {
	var parsed struct {
		Operations []planningParserOperation `json:"operations"`
	}
	if err := decodeContract(content, &parsed); err != nil {
		return nil, err
	}
	operations := make([]model.LedgerMutationOperation, 0, len(parsed.Operations))
	// Materialize each patch against preceding operations without mutating the caller.
	ledger.Tasks = append([]LedgerTask(nil), ledger.Tasks...)
	for _, value := range parsed.Operations {
		operation := model.LedgerMutationOperation{Kind: value.Kind, TargetID: value.TargetID,
			ReviewState: value.ReviewState, Rank: value.Rank, Rationale: value.Rationale}
		if len(value.Item) != 0 && !bytes.Equal(bytes.TrimSpace(value.Item), []byte("null")) {
			item := value.Item
			if value.Kind == model.LedgerMutationAmend {
				var err error
				item, err = mergePlanningPatch(ledger, value.TargetID, item)
				if err != nil {
					return nil, err
				}
			}
			operation.Item = &model.LedgerMutationItem{}
			if err := decodeContract(item, operation.Item); err != nil {
				return nil, err
			}
			if value.Kind == model.LedgerMutationAdd {
				if base := operation.Item.Base(); base != nil {
					base.Disposition = model.LedgerDispositionActive
					base.ReviewState = model.LedgerReviewProposed
				}
			}
		}
		// Repeated prose can extract the same typed delta twice. It has one
		// stable identity and one effect; normalize exact copies before creating
		// the domain mutation. Conflicting or merely similar operations remain
		// distinct and still face the unchanged domain validator.
		duplicate := false
		for _, prior := range operations {
			if reflect.DeepEqual(prior, operation) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			operations = append(operations, operation)
			if operation.Item != nil && operation.Item.Task != nil {
				switch operation.Kind {
				case model.LedgerMutationAdd:
					appendPlanningItem(&ledger, operation.Item)
				case model.LedgerMutationAmend:
					replacePlanningItem(&ledger, operation.TargetID, operation.Item)
				}
			}
		}
	}
	return operations, nil
}

func mergePlanningPatch(ledger EngineeringLedger, id model.LedgerItemID, raw json.RawMessage) (json.RawMessage, error) {
	var typed model.LedgerMutationItem
	if err := decodeContract(raw, &typed); err != nil {
		return nil, err
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err != nil || len(wrapper) != 1 {
		return nil, fmt.Errorf("planning amendment requires exactly one typed item")
	}
	retained, err := json.Marshal(retainedPlanningItem(ledger, id))
	if err != nil {
		return nil, err
	}
	var original map[string]map[string]json.RawMessage
	if err := json.Unmarshal(retained, &original); err != nil {
		return nil, err
	}
	for kind, payload := range wrapper {
		fields, exists := original[kind]
		if !exists {
			return nil, fmt.Errorf("planning amendment target %s does not exist with type %s", id, kind)
		}
		var patch map[string]json.RawMessage
		if err := json.Unmarshal(payload, &patch); err != nil || patch == nil {
			return nil, fmt.Errorf("planning amendment requires a field patch")
		}
		for name, value := range patch {
			switch name {
			case "id", "provenance", "disposition", "review_state", "rank", "required":
				return nil, fmt.Errorf("planning amendment cannot reconstruct code-owned field %s", name)
			}
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				continue
			}
			fields[name] = value
		}
	}
	return json.Marshal(original)
}

func retainedPlanningItem(ledger EngineeringLedger, id model.LedgerItemID) *model.LedgerMutationItem {
	for _, task := range ledger.Tasks {
		if task.ID == id {
			return &model.LedgerMutationItem{Task: &task}
		}
	}
	return nil
}

// Supporting spans add details to their owning task. Replacing an approved
// field is a change, not support; a later file/criterion list cannot erase the
// preceding span merely because it was projected in a separate bounded leaf.
func mergePlanningSupport(original, projected LedgerTask) (LedgerTask, error) {
	projected.Goal = original.Goal
	if original.Objective != "" {
		if projected.Objective != "" && projected.Objective != original.Objective {
			return projected, fmt.Errorf("task support cannot replace its owner's objective")
		}
		projected.Objective = original.Objective
	}
	projected.ReadFiles = appendDistinct(original.ReadFiles, projected.ReadFiles)
	projected.WriteFiles = appendDistinct(original.WriteFiles, projected.WriteFiles)
	projected.DeleteFiles = appendDistinct(original.DeleteFiles, projected.DeleteFiles)
	projected.Constraints = appendDistinct(original.Constraints, projected.Constraints)
	projected.OwnerContext = appendDistinct(original.OwnerContext, projected.OwnerContext)
	projected.FileResponsibilities = appendDistinct(original.FileResponsibilities, projected.FileResponsibilities)
	criteria := append([]model.TaskAcceptance(nil), original.Acceptance...)
	for _, criterion := range projected.Acceptance {
		found := false
		for index, prior := range criteria {
			if prior.ID != criterion.ID {
				continue
			}
			if prior.Text != criterion.Text {
				return projected, fmt.Errorf("task support reused acceptance identity %s for another criterion", criterion.ID)
			}
			criteria[index].Verification = appendDistinct(prior.Verification, criterion.Verification)
			found = true
			break
		}
		if !found {
			criteria = append(criteria, criterion)
		}
	}
	projected.Acceptance = criteria
	return projected, nil
}

func appendDistinct[T comparable](original, additional []T) []T {
	out := append([]T(nil), original...)
	for _, value := range additional {
		found := false
		for _, retained := range out {
			if retained == value {
				found = true
				break
			}
		}
		if !found {
			out = append(out, value)
		}
	}
	return out
}
