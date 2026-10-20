package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/sessionintent"
)

func TestTypedInteractionSkipsIntentInference(t *testing.T) {
	inputID := int64(9)
	for _, current := range []sessionintent.Context{{Items: []sessionintent.Item{}}, {Planning: true, Items: []sessionintent.Item{}}, {InputID: &inputID, Items: []sessionintent.Item{}}} {
		request := sessionintent.Request{Mode: sessionintent.Interact, Text: "  Please discuss approval; don't approve.\n"}
		result, err := interpretSessionUserIntent(t.Context(), request, current, func(context.Context, string) (string, error) {
			t.Fatal("typed content invoked intent inference")
			return "", nil
		})
		if err != nil {
			t.Fatal(err)
		}
		after, err := sessionintent.Apply(current, result.Patch)
		if err != nil {
			t.Fatal(after, err)
		}
		if current.InputID != nil {
			if after.NativeReply == nil || *after.NativeReply != request.Text {
				t.Fatal(after)
			}
		} else if after.Message == nil || after.Message.Text != request.Text || after.Message.Recipient != string(sessionintent.InteractionRecipient(current)) {
			t.Fatal(after)
		}
	}
}

func TestTypedHTTPDirectContentPersistsAndRejectsStaleExecution(t *testing.T) {
	url := os.Getenv("OMNI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	repository := freshCLIChatAPIRepository(t, url)
	route := codingagent.Route{Provider: "codex", Model: "fixture-model", Effort: "medium", ServiceTier: "default"}
	repository.WithCodingAgents(codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route})
	// Routes exist for job creation, but no provider connection exists: direct
	// transport must not invoke a semantic provider.
	server, err := NewServer(repository, nil, ServerOptions{LifecycleContext: t.Context(), HostDirectoryAccessRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	apiClient, err := client.New(httpServer.URL, 10*time.Second)
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
	state, err := repository.ChannelSessionState(t.Context(), channel.ID, link.Identity())
	if err != nil {
		t.Fatal(err)
	}
	text := "  Discuss /mode research literally.\nPreserve this text.  "
	requests := make([]sessionintent.Request, 2)
	for i := range requests {
		id, err := model.NewLifecycleOperationID()
		if err != nil {
			t.Fatal(err)
		}
		requests[i] = sessionintent.Request{OperationID: id, State: state, Mode: sessionintent.Interact, Recipient: sessionintent.Codex, Text: text}
		result, err := apiClient.InterpretUserMessage(t.Context(), channel, requests[i])
		if err != nil {
			t.Fatal(err)
		}
		after, err := sessionintent.Apply(sessionintent.Context{State: state, Items: []sessionintent.Item{}}, result.Patch)
		if err != nil || after.Message == nil || after.Message.Text != text || result.RequiresApproval {
			t.Fatal(result, err)
		}
	}
	first := requests[0]
	receipt, err := apiClient.ExecuteUserMessage(t.Context(), channel, link.Identity(), first.OperationID)
	if err != nil || receipt.JobID == 0 {
		t.Fatal(receipt, err)
	}
	snapshot, err := repository.ChannelSessionSnapshot(t.Context(), channel.ID, 10, link.Identity())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range snapshot.Transcript.Messages {
		if message.ID == receipt.MessageID {
			found = message.Content == text
		}
	}
	if !found {
		t.Fatal("persisted message lost exact owner text", snapshot.Transcript)
	}
	if _, err := apiClient.ExecuteUserMessage(t.Context(), channel, link.Identity(), requests[1].OperationID); !client.IsHTTPStatus(err, http.StatusConflict) {
		t.Fatal("submitted operation redirected into changed session", err)
	}
	// A committed operation still replays its own receipt, never the new recipient.
	again, err := apiClient.ExecuteUserMessage(t.Context(), channel, link.Identity(), first.OperationID)
	if err != nil || again.JobID != receipt.JobID || again.MessageID != receipt.MessageID {
		t.Fatal(again, err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "typed-planning-fixture")
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}
	item := model.LedgerItem{ID: "one", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "fixture", Revision: 1}, Rationale: "Requested task.", Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed}
	ledger := model.EngineeringLedger{ID: "fixture", Revision: 1, Tasks: []model.LedgerTask{{LedgerItem: item, Goal: text, WriteFiles: []string{"fixture.txt"}}}}
	var routes []queue.CodingLedgerRoute
	for _, role := range []string{"planner", "executor", "reviewer"} {
		routes = append(routes, queue.CodingLedgerRoute{Role: role, ConfigurationRevision: 1, Provider: route.Provider, Model: route.Model, ReasoningEffort: route.Effort, ServiceTier: route.ServiceTier})
	}
	created, err := repository.CreateCodingLedger(t.Context(), queue.CreateCodingLedgerCommand{JobID: receipt.JobID, OperationID: string(first.OperationID) + "_create", OwnerTurnID: "fixture", OwnerTurnText: text, Ledger: ledger, Routes: routes, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"one"})})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkCodingLedgerReviewReady(t.Context(), claim.Authority, created.LedgerID, created.Revision); err != nil {
		t.Fatal(err)
	}
	state, err = repository.ChannelSessionState(t.Context(), channel.ID, link.Identity())
	if err != nil {
		t.Fatal(err)
	}
	id, _ := model.NewLifecycleOperationID()
	// Planning ownership is known even without a displayed ledger revision.
	planning := sessionintent.Request{OperationID: id, State: state, Mode: sessionintent.Interact, Recipient: sessionintent.Planner, Text: "  Explain this plan.\nKeep the approved scope.  "}
	result, err := apiClient.InterpretUserMessage(t.Context(), channel, planning)
	if err != nil || len(result.Patch) != 1 || result.RequiresApproval {
		t.Fatal(result, err)
	}
	if _, err := apiClient.ExecuteUserMessage(t.Context(), channel, link.Identity(), id); err != nil {
		t.Fatal(err)
	}
	snapshot, err = repository.ChannelSessionSnapshot(t.Context(), channel.ID, 10, link.Identity())
	if err != nil || snapshot.ActiveJob == nil || snapshot.ActiveJob.Job.ID != receipt.JobID {
		t.Fatal(snapshot, err)
	}
	resumed := false
	for _, turn := range snapshot.Turns {
		if turn.Text == planning.Text && turn.Disposition == model.ChannelSessionTurnReplanned && turn.JobID == receipt.JobID && turn.Generation == claim.Authority.Generation+1 {
			resumed = true
		}
	}
	if !resumed {
		t.Fatal("typed planning reply bypassed the existing same-job planning continuation", snapshot.Turns)
	}
	retained, exists, err := repository.CurrentCodingLedgerForJob(t.Context(), receipt.JobID)
	if err != nil || !exists || retained.LedgerID != created.LedgerID || retained.Revision != created.Revision || retained.State != "proposed" {
		t.Fatal("direct planning content changed ledger approval authority", retained, err)
	}
}

func TestTypedStaleRecipientRejectedBeforeInference(t *testing.T) {
	for _, request := range []sessionintent.Request{
		{Mode: sessionintent.Interact, Recipient: sessionintent.Planner, Text: "exact text"},
		{Mode: sessionintent.Research, Text: "research"},
		{Mode: sessionintent.Ledger, Text: "approve"},
	} {
		_, err := interpretSessionUserIntent(t.Context(), request, sessionintent.Context{Pending: &sessionintent.Proposal{ID: "lifecycle_operation_pending"}}, func(context.Context, string) (string, error) {
			t.Fatal("invalid route reached inference")
			return "", nil
		})
		if err == nil {
			t.Fatal("invalid route accepted", request)
		}
	}
}

func TestTypedNativeChoiceStillInterpretsMeaning(t *testing.T) {
	inputID := int64(9)
	current := sessionintent.Context{InputID: &inputID, NativeChoices: []string{"Allow", "Deny"}, Items: []sessionintent.Item{}}
	calls := 0
	result, err := interpretSessionUserIntent(t.Context(), sessionintent.Request{Mode: sessionintent.Interact, Recipient: sessionintent.Native, Text: "No thanks"}, current, func(context.Context, string) (string, error) {
		calls++
		return `{"patch":[{"op":"replace","path":"/native_reply","value":"Deny"}]}`, nil
	})
	if err != nil || calls != 1 || len(result.Patch) != 1 || result.Patch[0].Path != "/native_reply" {
		t.Fatal(result, calls, err)
	}
}

func TestTypedNativeChoiceCannotApproveOrChangeWorkflow(t *testing.T) {
	inputID := int64(9)
	current := sessionintent.Context{InputID: &inputID, NativeChoices: []string{"Allow", "Deny"}, Items: []sessionintent.Item{}}
	for _, response := range []string{
		`{"patch":[{"op":"replace","path":"/open","value":false}]}`,
		`{"patch":[{"op":"replace","path":"/approval","value":true}]}`,
		`{"patch":[{"op":"replace","path":"/native_reply","value":"Other"}]}`,
	} {
		_, err := interpretSessionUserIntent(t.Context(), sessionintent.Request{Mode: sessionintent.Interact, Recipient: sessionintent.Native, Text: "No thanks"}, current, func(context.Context, string) (string, error) {
			return response, nil
		})
		if err == nil {
			t.Fatalf("constrained answer accepted invalid mutation: %s", response)
		}
	}
}

func TestExplicitInferRetainsIntentInterpretation(t *testing.T) {
	calls := 0
	result, err := interpretSessionUserIntent(t.Context(), sessionintent.Request{Mode: sessionintent.Infer, Text: "Show status"}, sessionintent.Context{Items: []sessionintent.Item{}}, func(context.Context, string) (string, error) {
		calls++
		return `{"patch":[{"op":"replace","path":"/status_visible","value":true}]}`, nil
	})
	if err != nil || calls != 1 || len(result.Patch) != 1 || result.Patch[0].Path != "/status_visible" {
		t.Fatal(result, calls, err)
	}
}

func TestTypedApprovalKeepsOnlyApprovalMeaning(t *testing.T) {
	current := sessionintent.Context{Items: []sessionintent.Item{}, Pending: &sessionintent.Proposal{ID: "lifecycle_operation_pending", Pitch: "Remove displayed item?"}}
	request := sessionintent.Request{Mode: sessionintent.Interact, Text: "Yes"}
	result, err := interpretSessionUserIntent(t.Context(), request, current, func(context.Context, string) (string, error) {
		return `{"patch":[{"op":"replace","path":"/approval","value":true}]}`, nil
	})
	if err != nil || len(result.Patch) != 1 || result.Patch[0].Path != "/approval" {
		t.Fatal(result, err)
	}
	_, err = interpretSessionUserIntent(t.Context(), request, current, func(context.Context, string) (string, error) {
		return `{"patch":[{"op":"replace","path":"/open","value":false}]}`, nil
	})
	if err == nil {
		t.Fatal("approval inference modified another field")
	}
}
