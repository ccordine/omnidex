package sessionintent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

// StateContract describes data, not callable actions. Shapes come from the same
// DTOs decoded by Validate; enum values use the persisted model constants.
func StateContract(c Context) (json.RawMessage, error) {
	schema, err := describeType(reflect.TypeOf(Snapshot{}))
	if err != nil {
		return nil, err
	}
	p := schema.Properties
	p["authority"].ReadOnly = true
	p["authority"].Description = "Persisted authority and reference evidence; immutable."
	p["items"].Items.Properties["id"].ReadOnly = true
	p["items"].Items.Properties["text"].ReadOnly = true
	review := p["items"].Items.Properties["review_state"]
	review.Enum = []any{model.LedgerReviewProposed, model.LedgerReviewAgreed, model.LedgerReviewNeedsDiscussion}
	review.Description = "Owner review: proposed is not yet agreed, agreed records agreement, needs_discussion records requested discussion. Agreement alone leaves plan_state and job_status unchanged."
	disposition := p["items"].Items.Properties["disposition"]
	disposition.Enum = []any{model.LedgerDispositionActive, model.LedgerDispositionTombstoned}
	disposition.Description = "active belongs to the plan; tombstoned removes the item while preserving its identity and review state."
	p["plan_state"].Enum = stateValues(c.PlanState, "frozen")
	p["plan_state"].Description = "proposed is editable; frozen commits the agreed plan and starts execution. Starting a proposed plan requires all remaining active items agreed, plan_state frozen and job_status running together."
	p["job_status"].Enum = stateValues(c.JobStatus, model.JobStatusRunning, model.JobStatusWaiting, model.JobStatusCanceled)
	p["job_status"].Description = "running executes or resumes the existing job; waiting_input interrupts it; canceled terminates it. A recoverable job resumes its interrupted planning or execution without changing the retained plan or item reviews."
	p["approval"].Description = "Response to authority.pending only: true confirms its exact stored proposal, false declines it. A reply confirming the pending proposal changes approval, not the proposed target fields again. Plan agreement is item review_state, not this field."
	if c.Pending == nil {
		p["approval"].Enum = []any{nil}
	}
	recipient := messageRecipient(c)
	p["message"].Description = "Deliver this user message as task content to the current workflow owner, separate from application state controls. Set the object with empty text and the current recipient. Code inserts the complete original user message unchanged; never copy, summarize or rewrite it."
	p["message"].Properties["text"].Enum = []any{""}
	p["message"].Properties["text"].Description = "Code supplies the original user message. Return an empty string."
	p["message"].Properties["recipient"].Enum = []any{recipient}
	p["message"].Properties["recipient"].Description = "Current workflow owner. Codex task content is delivered to planner during planning and codex otherwise."
	p["native_reply"].Description = "Answer to the current native question only; null when no answer is intended."
	if c.InputID == nil {
		p["native_reply"].Enum = []any{nil}
	} else if len(c.NativeChoices) > 0 {
		for _, v := range c.NativeChoices {
			p["native_reply"].Enum = append(p["native_reply"].Enum, v)
		}
		p["native_reply"].Enum = append(p["native_reply"].Enum, nil)
	}
	p["open"].Description = "Whether this client conversation stays open; closing it does not cancel the server job."
	p["status_visible"].Description = "Whether the owner requests the current authoritative job status."
	if !c.Planning || c.PlanState != "proposed" {
		p["items"].ReadOnly, p["plan_state"].ReadOnly = true, true
	}
	if c.JobID == 0 {
		p["job_status"].ReadOnly = true
	}
	schema.Description = "Required state fields remain present. Only item review_state/disposition are mutable within items; identities, text and item count are immutable. Proposed displayed plans alone are editable. A pending proposal is already stored, not yet applied. A confirmation cannot itself require confirmation. Unresolved conditions require clarification, not assumed satisfaction."
	return json.Marshal(schema)
}

func stateValues(values ...string) []any {
	result := []any{}
	seen := map[string]bool{}
	for _, value := range values {
		if !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	return result
}

func messageRecipient(c Context) string {
	if c.Planning {
		return "planner"
	}
	return "codex"
}

type stateSchema struct {
	Type        []string                `json:"type"`
	Properties  map[string]*stateSchema `json:"properties,omitempty"`
	Required    []string                `json:"required,omitempty"`
	Items       *stateSchema            `json:"items,omitempty"`
	Enum        []any                   `json:"enum,omitempty"`
	ReadOnly    bool                    `json:"readOnly,omitempty"`
	Description string                  `json:"description,omitempty"`
}

func describeType(t reflect.Type) (*stateSchema, error) {
	if t.Kind() == reflect.Pointer {
		s, err := describeType(t.Elem())
		if err == nil {
			s.Type = append(s.Type, "null")
		}
		return s, err
	}
	s := &stateSchema{}
	switch t.Kind() {
	case reflect.String:
		s.Type = []string{"string"}
	case reflect.Bool:
		s.Type = []string{"boolean"}
	case reflect.Slice:
		s.Type = []string{"array", "null"}
		var err error
		s.Items, err = describeType(t.Elem())
		if err != nil {
			return nil, err
		}
	case reflect.Struct:
		s.Type = []string{"object"}
		s.Properties = map[string]*stateSchema{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			var child *stateSchema
			if t == reflect.TypeOf(Snapshot{}) && name == "authority" {
				child = &stateSchema{Type: []string{"object"}, ReadOnly: true}
			} else {
				var err error
				child, err = describeType(f.Type)
				if err != nil {
					return nil, err
				}
			}
			s.Properties[name] = child
			s.Required = append(s.Required, name)
		}
	default:
		return nil, fmt.Errorf("unsupported session state type %s", t)
	}
	return s, nil
}
