package queue

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestLongCodingResponsePersistsBeyondChannelRowLimit(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	command := channelSessionCodingCommand(t, repository)
	command.CodingMode = codingagent.Agents
	initial, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "long-response-planner")
	if err != nil || planner == nil {
		t.Fatalf("claim planning: %v", err)
	}
	fixture := codingAgentPlanReviewFixture(t, repository, planner, initial.Job.Instruction)
	freezeCodingAgentPlanFixture(t, repository, initial.Job, fixture)
	executor, err := repository.ClaimNextStep(ctx, "long-response-executor")
	if err != nil || executor == nil {
		t.Fatalf("claim execution: %v", err)
	}
	task, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil || task == nil {
		t.Fatalf("select task: %v", err)
	}
	text := strings.Repeat("界", 16000) + strings.Repeat(" ", 40000) + "Finished."
	id, err := repository.CodingAgentEvidence(executor.Authority).Append(ctx, codingagent.Entry{
		Generation: initial.Job.CurrentGeneration, Phase: "execution_response", TaskID: task.ID,
		ResponseKind: codingagent.ExecutionDone, Notes: []string{text},
	})
	if err != nil {
		t.Fatalf("long final response rolled back execution evidence: %v", err)
	}
	var original string
	if err := pool.QueryRow(ctx, `SELECT payload_json->'notes'->>0 FROM evidence WHERE id=$1`, id).Scan(&original); err != nil || original != text {
		t.Fatalf("raw response changed: %v", err)
	}
	rows, err := pool.Query(ctx, `SELECT content FROM ai_channel_messages WHERE channel_id=$1 AND role='assistant' ORDER BY id`, command.ChannelID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	parts := []string{}
	for rows.Next() {
		var part string
		if err := rows.Scan(&part); err != nil {
			t.Fatal(err)
		}
		parts = append(parts, part)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(parts) < 2 {
		t.Fatal("long response was not projected into bounded messages")
	}
	var restored strings.Builder
	for index, part := range parts {
		restored.WriteString(strings.TrimPrefix(part, fmt.Sprintf("Response (part %d of %d)\n", index+1, len(parts))))
	}
	if restored.String() != text {
		t.Fatal("transcript lost or duplicated response bytes")
	}
}
