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
		t.Context(), fmt.Sprintf("scrum-ticket-action-%d", time.Now().UnixNano()), t.TempDir(), "")

	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(
		t.Context(), project.ID, "", "Exact ticket", "Use code-owned state.", "ready",
		json.RawMessage(`[{"id":"c1","text":"Persist once","done":false}]`),
		json.RawMessage(`["docs/contract.md"]`),
	)
	if err != nil {
		t.Fatal(err)
	}
	legacyPrompt := "Legacy opaque prompt."
	legacyTicket := "Legacy unproven ticket."
	tags := []string{"authority"}
	card, err = repository.UpdateScrumCardAtRevision(t.Context(), project.ID, card.ID, card.UpdatedAt, queue.ScrumCardRevisionPatch{
		Tags: &tags, CardPrompt: &legacyPrompt, CardTicket: &legacyTicket,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := true
	card, err = repository.MutateScrumCardItem(t.Context(), queue.ScrumCardItemMutation{
		ProjectID: project.ID, CardID: card.ID, ExpectedUpdatedAt: card.UpdatedAt,
		Collection: queue.ScrumCardTestCriteria, Action: queue.ScrumCardItemAdd, Text: "Tests pass",
	})
	if err != nil {
		t.Fatal(err)
	}
	var criteria []queue.ScrumCardItem
	if err := json.Unmarshal(card.TestCriteria, &criteria); err != nil || len(criteria) != 1 {
		t.Fatalf("decode created test criterion: items=%#v error=%v", criteria, err)
	}
	card, err = repository.MutateScrumCardItem(t.Context(), queue.ScrumCardItemMutation{
		ProjectID: project.ID, CardID: card.ID, ExpectedUpdatedAt: card.UpdatedAt,
		Collection: queue.ScrumCardTestCriteria, Action: queue.ScrumCardItemToggle,
		ItemID: criteria[0].ID, Done: &done,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{repo: repository, lifecycleContext: context.Background()}
	initialRevision := card.UpdatedAt.UTC().Format(time.RFC3339Nano)

	assembledResponse := postScrumCardTicketAction(t, server, project.ID, card.ID, map[string]any{
		"action": scrumCardTicketAssemble, "expected_updated_at": initialRevision,
	})
	if assembledResponse.Code != http.StatusOK {
		t.Fatalf("assemble status=%d body=%s", assembledResponse.Code, assembledResponse.Body.String())
	}
	assembled := decodeScrumCardTicketResponse(t, assembledResponse)
	if len(assembled.Chat) != 0 || assembled.ChatCount != 0 {
		t.Fatalf("ticket action returned unbounded card history: %+v", assembled)
	}
	for _, required := range []string{"# Exact ticket", "Use code-owned state.", "- [ ] Persist once", "- [x] Tests pass", "`docs/contract.md`", "- authority"} {
		if !strings.Contains(assembled.CardTicket, required) {
			t.Errorf("assembled ticket missing %q:\n%s", required, assembled.CardTicket)
		}
	}
	for _, legacy := range []string{"Legacy opaque prompt.", "Legacy unproven ticket."} {
		if strings.Contains(assembled.CardTicket, legacy) {
			t.Errorf("assembled ticket silently blessed legacy content %q:\n%s", legacy, assembled.CardTicket)
		}
	}
	if assembled.CardPrompt != "Legacy opaque prompt." {
		t.Fatalf("assemble should preserve display-only legacy prompt, got %q", assembled.CardPrompt)
	}
	if assembled.UpdatedAt == initialRevision {
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
		"action": scrumCardTicketApplyElaboration, "expected_updated_at": initialRevision,
		"elaboration": "This stale edit must not land.",
	})
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale status=%d body=%s", staleResponse.Code, staleResponse.Body.String())
	}
	afterStale, err := repository.GetScrumCard(t.Context(), project.ID, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterStale.CardTicket != assembled.CardTicket || afterStale.CardPrompt != "Legacy opaque prompt." {
		t.Fatalf("stale action mutated card: %#v", afterStale)
	}

	exactElaboration := "  Preserve the exact HTTP contract.\nKeep this tab:\t "
	appliedResponse := postScrumCardTicketAction(t, server, project.ID, card.ID, map[string]any{
		"action": scrumCardTicketApplyElaboration, "expected_updated_at": assembled.UpdatedAt,
		"elaboration": exactElaboration,
	})
	if appliedResponse.Code != http.StatusOK {
		t.Fatalf("apply elaboration status=%d body=%s", appliedResponse.Code, appliedResponse.Body.String())
	}
	applied := decodeScrumCardTicketResponse(t, appliedResponse)
	if applied.CardPrompt != exactElaboration ||
		!strings.Contains(applied.CardTicket, "## Elaboration\n\n"+exactElaboration+"\n") {
		t.Fatalf("elaboration-applied card=%#v", applied)
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
