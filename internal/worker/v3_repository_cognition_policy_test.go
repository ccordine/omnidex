package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/repository/cognitionenv"
)

type repositoryCognitionTestClient struct {
	mu           sync.Mutex
	model        string
	native       int
	output       int
	attestations int
	prepared     int
	generated    int
	cleaned      int
	plain        int
	prompts      []string
}

func (client *repositoryCognitionTestClient) Generate(context.Context, string, string) (string, error) {
	client.mu.Lock()
	client.plain++
	client.mu.Unlock()
	return "", fmt.Errorf("plain generation is forbidden for repository cognition")
}

func (client *repositoryCognitionTestClient) PrepareContextModel(
	_ context.Context,
	modelName string,
	prompt string,
) (llm.PreparedModel, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.prepared++
	return llm.PreparedModel{BaseModel: modelName, ContextModel: modelName, Prompt: prompt}, nil
}

func (client *repositoryCognitionTestClient) GeneratePrepared(
	_ context.Context,
	_ llm.PreparedModel,
) (string, error) {
	return "", fmt.Errorf("legacy prepared generation is forbidden for repository cognition")
}

func (client *repositoryCognitionTestClient) CleanupPreparedModel(llm.PreparedModel) {
	client.mu.Lock()
	client.cleaned++
	client.mu.Unlock()
}

func (*repositoryCognitionTestClient) Embedding(context.Context, string) ([]float64, error) {
	return nil, nil
}

func (*repositoryCognitionTestClient) RequireExactPreparedContract() error { return nil }

func (*repositoryCognitionTestClient) ValidateExactPreparedProvider(
	expected llm.ProviderIdentityExpectation,
) error {
	return llm.ValidateExactPreparedProviderExpectation(expected)
}

func (client *repositoryCognitionTestClient) ValidateExactPreparedContract(
	prepared llm.PreparedModel,
) error {
	if prepared.BaseModel != client.model || prepared.ContextModel != client.model ||
		prepared.Prompt == "" || prepared.ContextTokens != client.native ||
		prepared.MaxOutputTokens != client.output || prepared.PromptHint != llm.MinimalGeneratePrompt ||
		prepared.ResponseFormat != llm.ResponseFormatJSON || len(prepared.ResponseSchema) == 0 ||
		prepared.ThinkingEnabled || prepared.Temperature == nil || *prepared.Temperature != 0 ||
		prepared.ProviderIdentityExpectation == nil || prepared.ProviderObservationChallenge == "" {
		return fmt.Errorf("repository cognition prepared contract changed")
	}
	return nil
}

func repositoryCognitionDecisionFromEnvelope(prompt string) (string, error) {
	var envelope struct {
		EvidenceRefs []cognition.EvidenceRef `json:"evidence_refs"`
		Context      struct {
			Items []struct {
				Ref struct {
					URI           string `json:"uri"`
					ContentSHA256 string `json:"content_sha256"`
				} `json:"ref"`
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"items"`
		} `json:"projected_context"`
	}
	if err := json.Unmarshal([]byte(prompt), &envelope); err != nil {
		return "", err
	}
	var obligation cognition.Obligation
	var stage string
	var symbolRefs, inspectedRefs []string
	var rankedSymbol string
	var rankedRevision uint64
	var selected, initial cognition.EvidenceRef
	for _, item := range envelope.Context.Items {
		if item.Role == "task" {
			if err := json.Unmarshal([]byte(item.Content), &obligation); err != nil {
				return "", err
			}
		}
		if item.Role != "evidence" {
			continue
		}
		var state struct {
			Stage          string   `json:"stage"`
			DiscoveredRefs []string `json:"discovered_symbol_refs"`
			InspectedRefs  []string `json:"inspected_symbol_refs"`
		}
		isState := json.Unmarshal([]byte(item.Content), &state) == nil && state.Stage != ""
		var pack struct {
			Operation string `json:"operation"`
			Symbols   []struct {
				ID string `json:"id"`
			} `json:"symbols"`
		}
		isRankedPack := json.Unmarshal([]byte(item.Content), &pack) == nil &&
			pack.Operation == "semantic_excerpts" && len(pack.Symbols) > 0
		for _, ref := range envelope.EvidenceRefs {
			if ref.SHA256 != item.Ref.ContentSHA256 ||
				!strings.HasSuffix(item.Ref.URI, "/"+string(ref.ObservationID)) {
				continue
			}
			if isState && (selected.ObservationID == "" || ref.Revision.Number > selected.Revision.Number) {
				stage, symbolRefs, inspectedRefs = state.Stage, state.DiscoveredRefs, state.InspectedRefs
				selected = ref
			} else if ref.Revision.Number == 1 {
				initial = ref
			}
			if isRankedPack && ref.Revision.Number >= rankedRevision {
				rankedSymbol, rankedRevision = pack.Symbols[0].ID, ref.Revision.Number
			}
		}
	}
	if selected.ObservationID == "" {
		selected = initial
	}
	if obligation.ID == "" || selected.ObservationID == "" {
		return "", fmt.Errorf("repository cognition envelope omitted current task or state evidence")
	}
	kind := cognitionenv.ActionSearch
	arguments := []cognition.ActionArgument{}
	if stage == "searched" {
		kind = cognitionenv.ActionInspect
	} else if stage == "inspected" {
		kind = cognitionenv.ActionReferences
	}
	if kind != cognitionenv.ActionSearch {
		target := ""
		if stage == "searched" {
			target = rankedSymbol
		} else if len(inspectedRefs) > 0 {
			target = inspectedRefs[0]
		} else if len(symbolRefs) > 0 {
			target = symbolRefs[0]
		}
		if target == "" {
			return "", fmt.Errorf("repository cognition state omitted its opaque symbol reference")
		}
		arguments = append(arguments, cognition.ActionArgument{
			Name: cognitionenv.ArgumentSymbolRef, Value: target,
		})
	}
	request, err := cognition.NewActionRequest(kind, arguments)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(cognition.CognitionDecision{
		ObligationID: obligation.ID, Action: request,
		EvidenceRefs:   []cognition.EvidenceRef{selected},
		ExpectedEffect: "Acquire the next bounded repository investigation state.",
		Proposals:      []cognition.LedgerProposal{}, Attention: []cognition.AttentionRequest{},
	})
	return string(raw), err
}
