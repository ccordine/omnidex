package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

type desiredStateProductModelCall struct {
	Prompt, Response, Schema string
	Protocol                 llm.ExactPreparedProtocol
}

// desiredStateProductProvider is a deterministic, fixture-aware provider used
// only to exercise the checked-in production plumbing. Its responses make the
// run contaminated under the autonomy benchmark contract; no test using this
// provider is Gate D autonomy evidence.
type desiredStateProductProvider struct {
	mu    sync.Mutex
	calls []desiredStateProductModelCall
}

func (*desiredStateProductProvider) RequireExactPreparedContract() error { return nil }

func (*desiredStateProductProvider) ValidateExactPreparedProvider(
	expected llm.ProviderIdentityExpectation,
) error {
	return expected.Validate()
}

func (*desiredStateProductProvider) ValidateExactPreparedContract(prepared llm.PreparedModel) error {
	if prepared.ProviderIdentityExpectation == nil {
		return fmt.Errorf("product vertical provider requires exact identity authority")
	}
	return llm.ValidateResponseContract(prepared)
}

func (*desiredStateProductProvider) DiscoverProviderIdentityEvidence(
	_ context.Context,
	selection llm.ProviderIdentitySelection,
	challenge string,
) (llm.ObservedProviderIdentity, error) {
	return desiredStateProductObservedIdentity(selection, challenge)
}

func (provider *desiredStateProductProvider) GeneratePreparedExact(
	_ context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	response, schema, err := desiredStateProductResponse(prepared)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	provider.mu.Lock()
	provider.calls = append(provider.calls, desiredStateProductModelCall{
		Prompt: prepared.Prompt, Response: response, Schema: schema, Protocol: prepared.Protocol,
	})
	provider.mu.Unlock()
	return desiredStateProductGeneration(prepared, response)
}

func (provider *desiredStateProductProvider) Calls() []desiredStateProductModelCall {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return append([]desiredStateProductModelCall(nil), provider.calls...)
}

func desiredStateProductResponse(prepared llm.PreparedModel) (string, string, error) {
	schema := desiredStateProductSchemaConst(prepared.ResponseSchema)
	switch schema {
	case assemblyline.ConversationObjectiveKindSchemaV1:
		return fmt.Sprintf(`{"schema":%q,"kind":"workspace_mutation"}`, schema), schema, nil
	case assemblyline.RepositoryRequirementInterpretationSchemaV1:
		source, err := desiredStateProductRequirementSource(prepared.Prompt)
		if err != nil {
			return "", schema, err
		}
		response, err := json.Marshal(assemblyline.RepositoryRequirementInterpretation{
			Schema: schema, FeatureQuotes: []string{source},
		})
		return string(response), schema, err
	case assemblyline.DeclarationArtifactBoundarySchemaV1:
		return fmt.Sprintf(
			`{"schema":%q,"declaration_id":"DECLARATION_1","boundary":"independent_artifact"}`,
			schema,
		), schema, nil
	case assemblyline.KnownArtifactTruthSchemaV1:
		truth := assemblyline.KnownArtifactTruthNotApplicable
		if strings.Contains(prepared.Prompt, "must no longer exist") {
			truth = assemblyline.KnownArtifactMustBeAbsent
		}
		return fmt.Sprintf(
			`{"schema":%q,"truth":%q}`,
			schema, truth,
		), schema, nil
	case assemblyline.ArtifactCandidateSelectionSchemaV1:
		candidateID, err := desiredStateProductSelectDeclarationCandidate(
			prepared.Prompt, "Obsolete",
		)
		if err != nil {
			return "", schema, err
		}
		return fmt.Sprintf(`{"schema":%q,"candidate_id":%q}`, schema, candidateID), schema, nil
	case assemblyline.ArtifactHandlingSchemaV1:
		token, err := desiredStateProductPromptLine(prepared.Prompt, "FOCUSED_ARTIFACT: ")
		if err != nil {
			return "", schema, err
		}
		return fmt.Sprintf(
			`{"schema":%q,"token":%q,"handling":"must_be_absent"}`,
			schema, token,
		), schema, nil
	case "":
		if strings.Contains(prepared.Prompt, "EXACT_SIGNATURE:\nfunc Added() int") {
			return "func Added() int { return 2 }", schema, nil
		}
		return "", schema, fmt.Errorf("unexpected raw product vertical envelope")
	default:
		return "", schema, fmt.Errorf("unexpected product vertical response schema %q", schema)
	}
}

func desiredStateProductRequirementSource(prompt string) (string, error) {
	const marker = "CURRENT_REQUEST:\n"
	index := strings.LastIndex(prompt, marker)
	if index < 0 {
		return "", fmt.Errorf("product vertical repository requirements omitted current request")
	}
	source := strings.TrimSpace(prompt[index+len(marker):])
	if source == "" {
		return "", fmt.Errorf("product vertical repository requirement source is empty")
	}
	return source, nil
}

func desiredStateProductSelectDeclarationCandidate(
	prompt string,
	declarationName string,
) (string, error) {
	const marker = "BOUNDED_CANDIDATES:\n"
	index := strings.LastIndex(prompt, marker)
	if index < 0 {
		return "", fmt.Errorf("product vertical candidate selector omitted bounded evidence")
	}
	var candidates []assemblyline.ArtifactCandidateEvidence
	if err := json.Unmarshal([]byte(strings.TrimSpace(prompt[index+len(marker):])), &candidates); err != nil {
		return "", fmt.Errorf("decode product vertical bounded candidates: %w", err)
	}
	selected := ""
	for _, candidate := range candidates {
		for _, declaration := range candidate.Declarations {
			if !strings.Contains(declaration, declarationName) {
				continue
			}
			if selected != "" {
				return "", fmt.Errorf("product vertical declaration evidence is ambiguous")
			}
			selected = candidate.CandidateID
		}
	}
	if selected == "" {
		return "", fmt.Errorf("product vertical declaration evidence has no semantic match")
	}
	return selected, nil
}

func desiredStateProductSchemaConst(schema map[string]any) string {
	properties, _ := schema["properties"].(map[string]any)
	property, _ := properties["schema"].(map[string]any)
	value, _ := property["const"].(string)
	return value
}

func desiredStateProductPromptLine(prompt, prefix string) (string, error) {
	index := strings.Index(prompt, prefix)
	if index < 0 {
		return "", fmt.Errorf("product vertical envelope omitted %q", prefix)
	}
	value := prompt[index+len(prefix):]
	if end := strings.IndexByte(value, '\n'); end >= 0 {
		value = value[:end]
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("product vertical envelope contains empty %q", prefix)
	}
	return value, nil
}

func desiredStateProductObservedIdentity(
	selection llm.ProviderIdentitySelection,
	challenge string,
) (llm.ObservedProviderIdentity, error) {
	evidence, err := desiredStateProductIdentityEvidence(selection)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	expected, err := llm.DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	attestation, err := llm.NewProviderIdentityAttestation(
		expected, "exact backend evidence", "exact installed evidence", "exact runner evidence",
	)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	return llm.NewObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation, evidence, challenge,
	)
}

func desiredStateProductIdentityEvidence(
	selection llm.ProviderIdentitySelection,
) (llm.ProviderIdentityEvidence, error) {
	tokenizerRequest, err := llm.ExactProviderTokenizerRequestBytes(selection)
	if err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	preloadRequest, err := llm.ExactProviderPreloadRequestBytes(selection)
	if err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	digest := strings.Repeat("7", 64)
	installed := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":"Q4_K_M"}}]}`,
		selection.Model, selection.Model, digest,
	))
	tokenizer := []byte(`{"model_info":{"general.architecture":"qwen35","tokenizer.ggml.model":"gpt2","tokenizer.ggml.pre":"qwen35","tokenizer.ggml.add_eos_token":false,"tokenizer.ggml.add_padding_token":false,"tokenizer.ggml.tokens":null,"tokenizer.ggml.token_type":null,"tokenizer.ggml.merges":null}}`)
	runner := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":"Q4_K_M"},"context_length":%d}]}`,
		selection.Model, selection.Model, digest, selection.NativeContextLimit,
	))
	return llm.NewSuccessfulProviderIdentityEvidence(
		[]byte(`{"version":"0.24.0"}`), installed, tokenizerRequest, tokenizer,
		preloadRequest, []byte(`{"done":true}`), runner,
	)
}

func desiredStateProductGeneration(
	prepared llm.PreparedModel,
	content string,
) (llm.PreparedGeneration, error) {
	selection := llm.ProviderIdentitySelection{
		Model: prepared.ContextModel, NativeContextLimit: prepared.ContextTokens,
	}
	observed, err := desiredStateProductObservedIdentity(
		selection, prepared.ProviderObservationChallenge,
	)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	done := true
	body, err := json.Marshal(map[string]any{
		"model": prepared.ContextModel, "created_at": "2026-08-13T12:00:00Z",
		"response": content, "done": done, "done_reason": "stop",
		"total_duration": 10, "load_duration": 1, "prompt_eval_count": 1,
		"prompt_eval_duration": 2, "eval_count": 1, "eval_duration": 3,
	})
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	requestSHA, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	digest := sha256.Sum256(body)
	responseSHA := hex.EncodeToString(digest[:])
	return llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition: llm.ProviderRequestDispatched,
		Content:                    content, ProviderRequestSHA256: requestSHA, ProviderHTTPStatus: 200,
		ProviderResponseDisposition: llm.ProviderResponseSucceeded,
		ProviderResponseComplete:    true,
		ProviderContentEncoding:     llm.NewProviderContentEncodingEvidence(nil, false),
		ProviderResponseBytesKnown:  true, ProviderResponseSHA256: responseSHA,
		ProviderResponseBytes: int64(len(body)), ProviderResponseCaptureSHA256: responseSHA,
		ProviderResponseCapturedBytes: len(body), ProviderResponseCapture: body,
		ProviderResponseModel: prepared.ContextModel, ProviderDonePresent: true,
		ProviderDone: true, ProviderDoneReason: "stop", UsagePresent: true,
		Usage: llm.ProviderGenerationUsage{
			PromptEvalCount: 1, EvalCount: 1, TotalDurationNanos: 10,
			LoadDurationNanos: 1, PromptEvalDurationNanos: 2, EvalDurationNanos: 3,
		},
		ProviderObservation: observed.Observation, ProviderIdentityEvidence: observed.Evidence,
	}, nil
}
