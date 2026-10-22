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
	if err := json.Unmarshal(content, &parsed); err != nil {
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
			if err := json.Unmarshal(item, operation.Item); err != nil {
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
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, err
	}
	item := retainedPlanningItem(ledger, id)
	if item == nil {
		return nil, fmt.Errorf("planning amendment target %s does not exist", id)
	}
	retained, err := json.Marshal(item)
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
			continue
		}
		var patch map[string]json.RawMessage
		if err := json.Unmarshal(payload, &patch); err != nil {
			return nil, err
		}
		for name, value := range patch {
			switch name {
			case "goal", "objective", "owner_context", "constraints", "file_responsibilities", "acceptance", "read_files", "write_files", "delete_files":
			default:
				continue
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
