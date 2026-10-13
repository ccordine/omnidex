package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type terminalSemanticCall struct {
	id, step, attempt          int64
	kind, model, input, result string
	payload                    []byte
}

func readTerminalSemanticCalls(t *testing.T, ctx context.Context, runtime *Runtime, jobID int64) []terminalSemanticCall {
	t.Helper()
	rows, err := runtime.pool.Query(ctx, `SELECT calls.id,calls.step_id,calls.step_attempt,calls.work_kind,calls.model,calls.model_input,calls.work_input,receipts.candidate
		FROM llm_call_evidence AS calls
		JOIN llm_call_receipts AS receipts ON receipts.call_evidence_id=calls.id
		WHERE calls.job_id=$1 AND calls.scope='portable_semantic_worker' AND receipts.status='succeeded'
		ORDER BY calls.id LIMIT 257`, jobID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var calls []terminalSemanticCall
	for rows.Next() {
		var call terminalSemanticCall
		if err := rows.Scan(&call.id, &call.step, &call.attempt, &call.kind, &call.model, &call.input, &call.payload, &call.result); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, call)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(calls) > 256 {
		t.Fatal("terminal fixture exceeded its bounded journal observation budget")
	}
	return calls
}

// Expected content comes from the actual persisted request and response, plus
// the registered code rule. No intermediate requirement or model output is supplied.
func terminalSemanticFacts(call terminalSemanticCall) (opening, completed []string, candidate string, err error) {
	contract, err := assemblyline.SemanticUncertaintyContractForWorkKind(assemblyline.WorkKind(call.kind))
	if err != nil {
		return nil, nil, "", err
	}
	job := assemblyline.PortableJob{Schema: assemblyline.PortableJobSchemaV2, Kind: assemblyline.WorkKind(call.kind), Payload: call.payload}
	options, opaque, err := assemblyline.PortableJobOpaqueChoiceOptions(job)
	if err != nil {
		return nil, nil, "", err
	}
	question := contract.ExactQuestion
	if opaque {
		question, err = assemblyline.PortableJobOpaqueQuestion(job, call.input, options)
		if err != nil {
			return nil, nil, "", err
		}
	} else {
		options = []string{"Free-form output: " + contract.SingleResult}
	}
	var focus struct {
		Candidate json.RawMessage `json:"candidate"`
		Against   string          `json:"accepted_requirement"`
	}
	if err := json.Unmarshal(call.payload, &focus); err != nil {
		return nil, nil, "", err
	}
	if len(focus.Candidate) > 0 && focus.Candidate[0] == '"' {
		if err := json.Unmarshal(focus.Candidate, &candidate); err != nil {
			return nil, nil, "", err
		}
	}
	result := strings.TrimSpace(call.result)
	for _, option := range options {
		id, _, offered := strings.Cut(option, ". ")
		if offered && result == id {
			result = option
			break
		}
	}
	opening = append([]string{question, "Why: " + contract.DeterministicLimitation, "Options:", "Model: " + call.model}, options...)
	completed = []string{question, "Result: " + result, "Code rule: " + contract.DeterministicConsumer, "Model: " + call.model}
	for _, context := range []struct{ label, value string }{{"Candidate: ", candidate}, {"Compared with: ", focus.Against}} {
		if context.value != "" {
			opening = append(opening, context.label+context.value)
			completed = append(completed, context.label+context.value)
		}
	}
	return opening, completed, candidate, nil
}
