package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayContextSieveReceivesOnlyCodeOwnedSemanticInstruction(t *testing.T) {
	t.Parallel()
	required, err := assemblyline.NewContextCandidateAuthority(
		"scene_state", "CTX_1", strings.Repeat("current scene authority ", 60),
	)
	if err != nil {
		t.Fatal(err)
	}
	optional, err := assemblyline.NewContextCandidateAuthority(
		"conversation_exchange", "CTX_2", strings.Repeat("selected continuity ", 70),
	)
	if err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		name    string
		kind    roleplay.SimulationTurnInputKind
		raw     string
		visible string
		hidden  string
	}{
		{
			name: "deterministic action", kind: roleplay.SimulationTurnAction,
			raw:     `/give "ACTION_CONTEXT_COMMAND_SENTINEL"`,
			visible: "Continue the scene from the already-applied fictional events.",
			hidden:  "ACTION_CONTEXT_COMMAND_SENTINEL",
		},
		{
			name: "external research", kind: roleplay.SimulationTurnExternalCommand,
			raw:     `/research "RESEARCH_CONTEXT_COMMAND_SENTINEL"`,
			visible: "RESEARCH_CONTEXT_COMMAND_SENTINEL",
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			provider := &roleplayContextProviderProbe{contextSet: contextcompiler.CandidateSet{
				Required: []assemblyline.ContextCandidateAuthority{required},
				Optional: []assemblyline.ContextCandidateAuthority{optional},
			}}
			station := &scriptedConversationContextStation{
				relevantIDs:    []string{"CTX_2"},
				minimalContext: "Selected bounded continuity.",
			}
			authority := turnAuthority{
				JobID: 71, Instruction: fixture.raw, ChannelMode: model.ChannelModeRoleplay,
				RoleplayInputKind: fixture.kind,
			}
			authority, err := bindObjectiveModelInstruction(
				authority, assemblyline.ArtifactIdentityProvenance{},
			)
			if err != nil {
				t.Fatal(err)
			}
			compiled, calls, err := compileObjectiveTurnContext(
				context.Background(),
				model.Job{ID: authority.JobID, Pipeline: model.PipelineChat, Instruction: fixture.raw},
				authority, provider, station, nil, nil,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 || station.relevanceCalls != 1 ||
				station.minificationCalls != 1 {
				t.Fatalf(
					"calls=%d relevance=%d minification=%d",
					calls, station.relevanceCalls, station.minificationCalls,
				)
			}
			if compiled.Instruction != fixture.raw || provider.authority.Instruction != fixture.raw {
				t.Fatalf(
					"raw authority changed: compiled=%q provider=%q",
					compiled.Instruction, provider.authority.Instruction,
				)
			}
			if len(station.relevanceInputs) != 1 ||
				len(station.minificationInputs) != 1 {
				t.Fatalf("context station inputs were not captured exactly")
			}
			modelInputs, err := json.Marshal([]any{
				station.relevanceInputs[0], station.minificationInputs[0],
			})
			if err != nil {
				t.Fatal(err)
			}
			projected := string(modelInputs)
			if !strings.Contains(projected, fixture.visible) ||
				strings.Contains(projected, fixture.raw) ||
				(fixture.hidden != "" && strings.Contains(projected, fixture.hidden)) ||
				strings.Contains(projected, "/give") || strings.Contains(projected, "/research") {
				t.Fatalf("context model inputs leaked raw command authority: %s", projected)
			}
			if len(provider.terms) != 1 || provider.terms[0] != fixture.visible {
				t.Fatalf("fixed retrieval query=%#v want exact instruction %q", provider.terms, fixture.visible)
			}
		})
	}
}

func TestEmptyRoleplaySearchUniverseSkipsTermsButRanksFrozenOptionalAuthority(t *testing.T) {
	t.Parallel()
	optional, err := assemblyline.NewContextCandidateAuthority(
		"simulation_inventory", "CTX_1",
		"Inventory item rain cloak: a rain-dark traveling cloak (worn).",
	)
	if err != nil {
		t.Fatal(err)
	}
	provider := &roleplayContextProviderProbe{
		availability: contextcompiler.SearchUnavailable,
		contextSet: contextcompiler.CandidateSet{
			Optional: []assemblyline.ContextCandidateAuthority{optional},
		},
	}
	station := &scriptedConversationContextStation{
		relevantIDs: []string{"CTX_1"},
	}
	authority := turnAuthority{
		JobID: 72, Instruction: "I pull it tighter.", ChannelMode: model.ChannelModeRoleplay,
		RoleplayInputKind: roleplay.SimulationTurnProse,
	}
	authority, err = bindObjectiveModelInstruction(
		authority, assemblyline.ArtifactIdentityProvenance{},
	)
	if err != nil {
		t.Fatal(err)
	}
	compiled, calls, err := compileObjectiveTurnContext(
		context.Background(),
		model.Job{ID: authority.JobID, Pipeline: model.PipelineChat, Instruction: authority.Instruction},
		authority, provider, station, nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || provider.availabilityCalls != 1 || provider.contextCalls != 1 ||
		station.relevanceCalls != 1 ||
		provider.terms == nil || len(provider.terms) != 0 ||
		len(station.relevanceInputs) != 1 ||
		len(compiled.Context.Capsules) != 1 ||
		compiled.Context.Capsules[0].Content != optional.Content {
		t.Fatalf(
			"calls=%d availability/retrieval=%d/%d relevance=%d context=%#v",
			calls, provider.availabilityCalls, provider.contextCalls,
			station.relevanceCalls, compiled.Context,
		)
	}
}

func TestDeterministicRoleplayActionDoesNotManufactureSearchTerms(t *testing.T) {
	t.Parallel()
	provider := &roleplayContextProviderProbe{availability: contextcompiler.SearchAvailable}
	authority := turnAuthority{
		JobID: 73, Instruction: `/give "iron key"`, ChannelMode: model.ChannelModeRoleplay,
		RoleplayInputKind: roleplay.SimulationTurnAction,
	}
	directive, err := resolveRoleplayTurnRetrieval(
		t.Context(), model.Job{ID: 73, Pipeline: model.PipelineChat, Instruction: authority.Instruction},
		authority, provider,
		roleplay.SimulationTurnAuthority{InputKind: roleplay.SimulationTurnAction},
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider.availabilityCalls != 0 ||
		directive.Availability != contextcompiler.SearchUnavailable {
		t.Fatalf(
			"availability calls=%d directive=%#v",
			provider.availabilityCalls, directive,
		)
	}
}
