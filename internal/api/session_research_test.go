package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/sessionintent"
)

func TestResearchHTTPForkReplayReconnectAndWorkspaceBinding(t *testing.T) {
	url := os.Getenv("OMNI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	repo := freshCLIChatAPIRepository(t, url)
	route := codingagent.Route{Provider: "codex", Model: "fixture-model", Effort: "medium", ServiceTier: "default"}
	repo.WithCodingAgents(codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route})
	server, err := NewServer(repo, nil, ServerOptions{LifecycleContext: t.Context(), HostDirectoryAccessRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	apiClient, err := client.New(httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := projectroot.OpenDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	link, err := apiClient.OpenWorkspace(t.Context(), directory, strings.Repeat("1", 32))
	if err != nil {
		t.Fatal(err)
	}
	defer link.Close()
	channel, err := apiClient.BootstrapCLIChatSession(t.Context(), directory.Path(), link.Identity())
	if err != nil {
		t.Fatal(err)
	}
	id, _ := model.NewLifecycleOperationID()
	_, err = repo.SubmitChannelSessionTurn(t.Context(), queue.ChannelSessionTurnCommand{OperationID: id, ChannelID: channel.ID, WorkspaceRoot: directory.Path(), WorkspaceIdentity: link.Identity(), CodingMode: codingagent.Agents, Text: "Retain the source session"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := repo.ChannelSessionState(t.Context(), channel.ID, link.Identity())
	if err != nil {
		t.Fatal(err)
	}
	forkID, _ := model.NewLifecycleOperationID()
	request := sessionintent.ResearchRequest{OperationID: forkID, ConversationID: forkID, WorkspaceIdentity: link.Identity(), Text: "Investigate the selected message", Fork: &sessionintent.ResearchSeed{SourceChannel: channel.ID, MessageIDs: []int64{*before.LatestMessageID}}}
	// Seed only the native answer; HTTP replay must return durable code-owned origin.
	_, err = repo.Research(t.Context(), channel.ID, request, func(context.Context, sessionintent.ResearchContext) (string, json.RawMessage, error) {
		return "Retained finding", json.RawMessage(`[]`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := apiClient.Research(t.Context(), channel, request)
	if err != nil || replay.Fork == nil || replay.Fork.Messages[0].Text != "Retain the source session" {
		t.Fatal(replay, err)
	}
	// Cross the old 40-turn cutoff before reconnecting and sharing the oldest
	// finding. Real client retrieval must traverse bounded HTTP pages.
	for range 40 {
		followup := request
		followup.OperationID, _ = model.NewLifecycleOperationID()
		followup.Fork = nil
		_, err := repo.Research(t.Context(), channel.ID, followup, func(context.Context, sessionintent.ResearchContext) (string, json.RawMessage, error) {
			return "Later finding", json.RawMessage(`[]`), nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	retainedHistory, err := repo.ResearchConversationHistory(t.Context(), channel.ID, link.Identity(), forkID)
	if err != nil || len(retainedHistory) != 41 || retainedHistory[0].OperationID != forkID {
		t.Fatal("repository retrieval lost oldest finding", retainedHistory, err)
	}
	for _, check := range []struct {
		offset string
		status int
		count  int
	}{{"0", 200, 40}, {"40", 200, 1}, {"41", 200, 0}, {"-1", 400, 0}, {"invalid", 400, 0}} {
		response, err := httpServer.Client().Get(httpServer.URL + "/v1/channels/" + string(channel.ID) + "/session/research?workspace_identity=" + link.Identity() + "&conversation_id=" + string(forkID) + "&offset=" + check.offset)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != check.status {
			response.Body.Close()
			t.Fatalf("offset %s: status %d, want %d", check.offset, response.StatusCode, check.status)
		}
		if check.status == 200 {
			var page []sessionintent.ResearchTurn
			err := json.NewDecoder(response.Body).Decode(&page)
			if err != nil || len(page) != check.count {
				response.Body.Close()
				t.Fatalf("offset %s: got %d turns, want %d: %v", check.offset, len(page), check.count, err)
			}
		}
		response.Body.Close()
	}
	reconnected, err := client.New(httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	history, err := reconnected.ResearchConversationHistory(t.Context(), channel, link.Identity(), forkID)
	if err != nil || len(history) != 41 || history[0].OperationID != forkID || history[0].Fork.MessageIDs[0] != *before.LatestMessageID {
		t.Fatal(history, err)
	}
	main, err := reconnected.ResearchHistory(t.Context(), channel, link.Identity())
	if err != nil || len(main) != 0 {
		t.Fatal(main, err)
	}
	request.WorkspaceIdentity = "wrong-workspace"
	if _, err := apiClient.Research(t.Context(), channel, request); err == nil {
		t.Fatal("wrong workspace accepted")
	}
	after, err := repo.ChannelSessionState(t.Context(), channel.ID, link.Identity())
	if err != nil || !after.Equal(before) {
		t.Fatal("Research mutated coding authority", after, err)
	}
	// Explicitly sharing a retained finding enters the existing planner continuation.
	claim, err := repo.ClaimNextStep(t.Context(), "research-share-source-planner")
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}
	item := model.LedgerItem{ID: "one", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "fixture", Revision: 1}, Rationale: "Requested task", Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed}
	ledger := model.EngineeringLedger{ID: "fixture", Revision: 1, Tasks: []model.LedgerTask{{LedgerItem: item, Goal: "Retain the source session", WriteFiles: []string{"fixture.txt"}}}}
	var routes []queue.CodingLedgerRoute
	for _, role := range []string{"planner", "executor", "reviewer"} {
		routes = append(routes, queue.CodingLedgerRoute{Role: role, ConfigurationRevision: 1, Provider: route.Provider, Model: route.Model, ReasoningEffort: route.Effort, ServiceTier: route.ServiceTier})
	}
	created, err := repo.CreateCodingLedger(t.Context(), queue.CreateCodingLedgerCommand{JobID: claim.Job.ID, OperationID: string(id) + "_create", OwnerTurnID: "fixture", OwnerTurnText: "Retain the source session", Ledger: ledger, Routes: routes, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"one"})})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkCodingLedgerReviewReady(t.Context(), claim.Authority, created.LedgerID, created.Revision); err != nil {
		t.Fatal(err)
	}
	state, err := repo.ChannelSessionState(t.Context(), channel.ID, link.Identity())
	if err != nil {
		t.Fatal(err)
	}
	origin, err := json.Marshal(struct {
		Source       model.ChannelID            `json:"source_session"`
		Conversation model.LifecycleOperationID `json:"research_conversation"`
		Finding      model.LifecycleOperationID `json:"finding"`
		MessageIDs   []int64                    `json:"message_ids"`
		Text         string                     `json:"selected_finding"`
	}{channel.ID, forkID, replay.OperationID, replay.Fork.MessageIDs, replay.Answer})
	if err != nil {
		t.Fatal(err)
	}
	shareID, _ := model.NewLifecycleOperationID()
	share := sessionintent.Request{OperationID: shareID, State: state, Mode: sessionintent.Interact, Recipient: sessionintent.Planner, Text: "Research finding shared as planning input.\n" + string(origin)}
	result, err := reconnected.InterpretUserMessage(t.Context(), channel, share)
	if err != nil || result.RequiresApproval || len(result.Patch) != 1 {
		t.Fatal(result, err)
	}
	receipt, err := reconnected.ExecuteUserMessage(t.Context(), channel, link.Identity(), shareID)
	if err != nil || receipt.JobID != claim.Job.ID {
		t.Fatal(receipt, err)
	}
	snapshot, err := repo.ChannelSessionSnapshot(t.Context(), channel.ID, 10, link.Identity())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, turn := range snapshot.Turns {
		if turn.Text == share.Text && turn.Disposition == model.ChannelSessionTurnReplanned && turn.JobID == claim.Job.ID && turn.Generation == claim.Authority.Generation+1 {
			found = true
		}
	}
	if !found {
		t.Fatal("planner continuation lost selected finding/origin", snapshot.Turns)
	}
	retained, exists, err := repo.CurrentCodingLedgerForJob(t.Context(), claim.Job.ID)
	if err != nil || !exists || retained.LedgerID != created.LedgerID || retained.Revision != created.Revision || retained.State != "proposed" {
		t.Fatal("sharing amended executable authority", retained, err)
	}

}
