package codingagent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const ParserBlockBytes = 2048

// ParserStep is a code-selected semantic leaf, never a model-authored workflow.
// Source and neighbouring context refer only to the completed final response.
type ParserStep struct {
	Stage     string             `json:"stage"`
	BlockID   string             `json:"block_id"`
	Source    string             `json:"source"`
	Before    string             `json:"before,omitempty"`
	After     string             `json:"after,omitempty"`
	Category  string             `json:"category,omitempty"`
	Criterion string             `json:"criterion,omitempty"`
	Ledger    *EngineeringLedger `json:"ledger,omitempty"`
	NewIDs    map[string]string  `json:"new_ids,omitempty"`
	OwnerTurn string             `json:"owner_turn,omitempty"`
}

type ParserPart struct {
	Text   string `json:"text"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

type ParserBlock struct {
	ID    string `json:"id"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

// PartitionParserSource accounts for every byte without summarizing or guessing
// meaning. Long lines split only at UTF-8 boundaries; no piece is truncated.
func PartitionParserSource(source string) ([]ParserBlock, error) {
	if strings.TrimSpace(source) == "" || !utf8.ValidString(source) {
		return nil, fmt.Errorf("parser requires a nonblank UTF-8 final response")
	}
	blocks := []ParserBlock{}
	for start := 0; start < len(source); {
		end := min(start+ParserBlockBytes, len(source))
		if blank := strings.Index(source[start:end], "\n\n"); blank >= 0 {
			end = start + blank + 2
		} else if end < len(source) {
			if line := strings.LastIndexByte(source[start:end], '\n'); line >= 0 {
				end = start + line + 1
			}
		}
		for end < len(source) && !utf8.RuneStart(source[end]) {
			end--
		}
		blocks = append(blocks, ParserBlock{ID: fmt.Sprintf("block_%d", len(blocks)+1), Start: start, End: end, Text: source[start:end]})
		start = end
	}
	return blocks, nil
}

type parserSieve struct {
	call   Assignment
	event  func(NativeEvent) error
	run    func(context.Context, Assignment, func(NativeEvent) error) (Result, error)
	last   Result
	blocks []ParserBlock
	parts  [][]ParserPart
}

// RunParserSieve visits every source block and then every admitted planning
// statement or review criterion. Only the existing provider executes leaves.
// Returned native identities belong to the last real leaf, never invented calls.
func RunParserSieve(ctx context.Context, call Assignment, event func(NativeEvent) error, run func(context.Context, Assignment, func(NativeEvent) error) (Result, error)) (Result, error) {
	result := Result{Provider: call.Route.Provider, Model: call.Route.Model}
	if call.ParserStep != nil || call.Role.IsNative() || event == nil || run == nil {
		return result, fmt.Errorf("response sieve requires one parser role, original source and evidence sink")
	}
	if err := call.ValidateTaskContext(); err != nil {
		return result, err
	}
	source := call.Response
	if call.Role == PlanningInitial || call.Role == PlanningMutation {
		source = call.PlanningResponse
	}
	blocks, err := PartitionParserSource(source)
	if err != nil {
		return result, err
	}
	s := parserSieve{call: call, event: event, run: run, last: result, blocks: blocks}
	if err := s.receipt("source_partition", map[string]any{"sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(source))), "blocks": blocks}); err != nil {
		return result, err
	}
	for index, block := range blocks {
		parts := []ParserPart{{Text: block.Text, Kind: "ignore", Reason: "whitespace"}}
		if strings.TrimSpace(block.Text) != "" {
			var scan parserScanResult
			if err := s.leaf(ctx, s.step(index, "scan"), &scan); err != nil {
				return s.failed(err)
			}
			parts, err = parserScanParts(call.Role, block.Text, scan)
			if err != nil {
				return s.failed(err)
			}
		}
		if err := validateParserParts(call.Role, block.Text, parts); err != nil {
			return s.failed(fmt.Errorf("%s: %w", block.ID, err))
		}
		s.parts = append(s.parts, parts)
		if err := s.receipt("block_dispositions", map[string]any{"block_id": block.ID, "parts": parts}); err != nil {
			return s.failed(err)
		}
	}
	var content json.RawMessage
	switch call.Role {
	case PlanningInitial, PlanningMutation:
		content, err = s.planning(ctx)
	case ExecutionResponse:
		content, err = json.Marshal(map[string]any{"kind": s.outcome()})
	case ReviewResponse:
		content, err = s.review(ctx)
	default:
		err = fmt.Errorf("unknown response parser role %q", call.Role)
	}
	if err != nil {
		return s.failed(err)
	}
	if err := s.receipt("assembled", map[string]any{"content": content, "thread_id": s.last.ThreadID, "turn_id": s.last.TurnID}); err != nil {
		return s.failed(err)
	}
	s.last.Content, s.last.Completed = content, true
	return s.last, nil
}

func (s *parserSieve) step(index int, stage string) ParserStep {
	step := ParserStep{Stage: stage, BlockID: s.blocks[index].ID, Source: s.blocks[index].Text}
	for _, fact := range s.call.ConstructionFacts {
		if fact.Kind == "bounded_planning_context" {
			var payload struct {
				OwnerTurn PlanningTurn `json:"owner_turn"`
			}
			if json.Unmarshal([]byte(fact.Quote), &payload) == nil {
				step.OwnerTurn = payload.OwnerTurn.Text
			}
		}
	}
	if index > 0 {
		step.Before = s.blocks[index-1].Text
	}
	if index+1 < len(s.blocks) {
		step.After = s.blocks[index+1].Text
	}
	return step
}

func (s *parserSieve) leaf(ctx context.Context, step ParserStep, target any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	call := s.call
	call.Context, call.ParserStep = "", &step
	call.PlanningResponse, call.Response = "", ""
	if call.Role.IsPlanning() {
		call.PlanningResponse = step.Source
	} else {
		call.Response = step.Source
	}
	if err := s.receipt("leaf_opened", step); err != nil {
		return err
	}
	frames := 0
	result, err := s.run(ctx, call, func(event NativeEvent) error {
		if len(event.Frame) > 0 {
			frames++
		}
		return s.event(event)
	})
	s.last = result
	if err != nil {
		return fmt.Errorf("%s/%s: %w", step.BlockID, step.Stage, err)
	}
	if err := result.ValidateFor(call.Role, call.Route); err != nil {
		return err
	}
	if frames == 0 {
		return fmt.Errorf("semantic parser leaf has no original provider receipt")
	}
	if err := ValidateParserLeafResponse(call, result.Content); err != nil {
		return err
	}
	return decodeContract(result.Content, target)
}

func ValidateParserLeafResponse(call Assignment, content json.RawMessage) error {
	if call.ParserStep == nil {
		return fmt.Errorf("leaf validation requires a code-selected step")
	}
	switch call.ParserStep.Stage {
	case "scan":
		_, err := decodeParserScan(call, content)
		return err
	case "review_criterion":
		var criterion parserCriterionResult
		if err := decodeContract(content, &criterion); err != nil {
			return err
		}
		return validateParserCriterion(call.ParserStep.Source, criterion)
	case "planning_unit":
		var parsed parserPlanningProjection
		if err := decodeContract(content, &parsed); err != nil {
			return err
		}
		if parsed.Operations == nil {
			return fmt.Errorf("planning projection requires an operations array")
		}
		if len(parsed.ExistingConstraintIDs) > 0 && call.ParserStep.Category != "project_constraint" {
			return fmt.Errorf("existing global policy references crossed a different semantic leaf")
		}
		return nil
	default:
		return fmt.Errorf("unknown semantic parser step")
	}
}

func (s *parserSieve) receipt(stage string, value any) error {
	frame, err := json.Marshal(map[string]any{"processor": "response_sieve", "stage": stage, "value": value})
	if err != nil {
		return err
	}
	return s.event(NativeEvent{Frame: frame})
}

func (s *parserSieve) failed(err error) (Result, error) {
	s.last.Completed = false
	s.last.Content = nil
	return s.last, err
}

func validateParserParts(role Role, source string, parts []ParserPart) error {
	if len(parts) == 0 {
		return fmt.Errorf("source block has no dispositions")
	}
	var recovered strings.Builder
	for _, part := range parts {
		if part.Text == "" || strings.TrimSpace(part.Reason) == "" || !contains(parserPartKinds(role), part.Kind) {
			return fmt.Errorf("invalid or unexplained source disposition")
		}
		if part.Kind == "unknown" {
			return fmt.Errorf("unresolved source meaning: %s", part.Reason)
		}
		recovered.WriteString(part.Text)
	}
	if recovered.String() != source {
		return fmt.Errorf("source dispositions dropped, duplicated, reordered or rewrote text")
	}
	return nil
}
