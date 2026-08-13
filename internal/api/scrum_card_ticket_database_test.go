package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func TestPostgresScrumCardTicketActionsAreDeterministicAndRevisionBound(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(t.Context(), loadAPITestMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		t.Context(), fmt.Sprintf("scrum-ticket-action-%d", time.Now().UnixNano()), t.TempDir(), "", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(
		t.Context(), project.ID, "", "Exact ticket", "Use code-owned state.", "ready",
		json.RawMessage(`[{"id":"c1","text":"Persist once","done":false}]`),
		json.RawMessage(`["docs/contract.md"]`), json.RawMessage(`[]`),
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err = repository.UpdateScrumCard(t.Context(), project.ID, card.ID, map[string]any{
		"tags":          json.RawMessage(`["authority"]`),
		"test_criteria": json.RawMessage(`[{"id":"t1","text":"Tests pass","done":true}]`),
		"card_prompt":   "Legacy opaque prompt.",
		"card_ticket":   "Legacy unproven ticket.",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{repo: repository, lifecycleContext: context.Background()}
	initialRevision := card.UpdatedAt.UTC().Format(time.RFC3339Nano)

	generatedResponse := postScrumCardTicketAction(t, server, project.ID, card.ID, map[string]any{
		"action": scrumCardTicketGenerate, "expected_updated_at": initialRevision,
	})
	if generatedResponse.Code != http.StatusOK {
		t.Fatalf("generate status=%d body=%s", generatedResponse.Code, generatedResponse.Body.String())
	}
	generated := decodeScrumCardTicketResponse(t, generatedResponse)
	for _, required := range []string{"# Exact ticket", "Use code-owned state.", "- [ ] Persist once", "- [x] Tests pass", "`docs/contract.md`", "- authority"} {
		if !strings.Contains(generated.CardTicket, required) {
			t.Errorf("generated ticket missing %q:\n%s", required, generated.CardTicket)
		}
	}
	for _, legacy := range []string{"Legacy opaque prompt.", "Legacy unproven ticket."} {
		if strings.Contains(generated.CardTicket, legacy) {
			t.Errorf("generated ticket silently blessed legacy content %q:\n%s", legacy, generated.CardTicket)
		}
	}
	if generated.CardPrompt != "Legacy opaque prompt." {
		t.Fatalf("generate should preserve display-only legacy prompt, got %q", generated.CardPrompt)
	}
	if generated.UpdatedAt == initialRevision {
		t.Fatal("ticket mutation did not return a new authoritative card revision")
	}
	var jobCount int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM jobs`).Scan(&jobCount); err != nil {
		t.Fatal(err)
	}
	if jobCount != 0 {
		t.Fatalf("deterministic ticket action created %d inference jobs", jobCount)
	}

	staleResponse := postScrumCardTicketAction(t, server, project.ID, card.ID, map[string]any{
		"action": scrumCardTicketElaborate, "expected_updated_at": initialRevision,
		"elaboration": "This stale edit must not land.",
	})
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale status=%d body=%s", staleResponse.Code, staleResponse.Body.String())
	}
	afterStale, err := repository.GetScrumCard(t.Context(), project.ID, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterStale.CardTicket != generated.CardTicket || afterStale.CardPrompt != "Legacy opaque prompt." {
		t.Fatalf("stale action mutated card: %#v", afterStale)
	}

	elaboratedResponse := postScrumCardTicketAction(t, server, project.ID, card.ID, map[string]any{
		"action": scrumCardTicketElaborate, "expected_updated_at": generated.UpdatedAt,
		"elaboration": "Preserve the exact HTTP contract.",
	})
	if elaboratedResponse.Code != http.StatusOK {
		t.Fatalf("elaborate status=%d body=%s", elaboratedResponse.Code, elaboratedResponse.Body.String())
	}
	elaborated := decodeScrumCardTicketResponse(t, elaboratedResponse)
	if elaborated.CardPrompt != "Preserve the exact HTTP contract." ||
		!strings.Contains(elaborated.CardTicket, "## Elaboration\n\nPreserve the exact HTTP contract.") {
		t.Fatalf("elaborated card=%#v", elaborated)
	}
}

func postScrumCardTicketAction(
	t *testing.T,
	server *Server,
	projectID int64,
	cardID string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/v1/scrum/cards/%s/card-ticket?project_id=%d", cardID, projectID),
		bytes.NewReader(raw),
	)
	response := httptest.NewRecorder()
	server.handleScrumCardByID(response, request)
	return response
}

func decodeScrumCardTicketResponse(t *testing.T, response *httptest.ResponseRecorder) ScrumCard {
	t.Helper()
	var payload struct {
		Card ScrumCard `json:"card"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Card
}
