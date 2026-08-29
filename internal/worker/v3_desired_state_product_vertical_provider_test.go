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
	Prompt, Response string
	Kind             assemblyline.WorkKind
	Protocol         llm.ExactPreparedProtocol
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
	if selection.ProfilePolicy != "" {
		return llm.ObservedProviderIdentity{}, fmt.Errorf(
			"ordinary product provider received non-strict profile policy %q",
			selection.ProfilePolicy,
		)
	}
	return desiredStateProductObservedIdentity(selection, challenge)
}

func (provider *desiredStateProductProvider) GeneratePreparedExact(
	_ context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	response, kind, err := desiredStateProductResponse(prepared)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	provider.mu.Lock()
	provider.calls = append(provider.calls, desiredStateProductModelCall{
		Prompt: prepared.Prompt, Response: response, Kind: kind, Protocol: prepared.Protocol,
	})
	provider.mu.Unlock()
	return desiredStateProductGeneration(prepared, response)
}

func (provider *desiredStateProductProvider) Calls() []desiredStateProductModelCall {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return append([]desiredStateProductModelCall(nil), provider.calls...)
}

func desiredStateProductResponse(
	prepared llm.PreparedModel,
) (string, assemblyline.WorkKind, error) {
	prompt := prepared.Prompt
	switch {
	case strings.Contains(prompt, "Classify one exact user instruction"):
		return string(assemblyline.ObjectiveKindWorkspaceMutation),
			assemblyline.WorkConversationObjectiveKind, nil
	case strings.Contains(prompt, "Answer one semantic coverage relation: is there one necessary missing-fact question"):
		return assemblyline.ApplicationNoUncoveredContextNeed,
			assemblyline.WorkApplicationContextNeedCoverage, nil
	case strings.Contains(prompt, "Answer one semantic relation: does the immutable existing-repository request"):
		input, err := desiredStateProductRequirementProjection(prompt)
		if err != nil {
			return "", assemblyline.WorkRepositoryRequirementCoverage, err
		}
		if input.AcceptedCount == 0 {
			return assemblyline.RepositoryRequirementRemains,
				assemblyline.WorkRepositoryRequirementCoverage, nil
		}
		return assemblyline.RepositoryNoUncoveredRequirement,
			assemblyline.WorkRepositoryRequirementCoverage, nil
	case strings.Contains(prompt, "Return one explicit workspace-change requirement"):
		source, err := desiredStateProductRequirementSource(prompt)
		return source, assemblyline.WorkRepositoryRequirement, err
	case strings.Contains(prompt, "EXACT_GO_SIGNATURE:"):
		return string(assemblyline.DeclarationBoundaryIndependentArtifact),
			assemblyline.WorkDeclarationArtifactBoundary, nil
	case strings.Contains(prompt, "does the exact requirement explicitly require one semantic artifact established by repository authority"):
		relation := assemblyline.RepositoryArtifactAbsenceNotExplicit
		if strings.Contains(prompt, "must no longer exist") {
			relation = assemblyline.RepositoryArtifactMustBeAbsent
		}
		return string(relation), assemblyline.WorkRepositoryArtifactAbsence, nil
	case strings.Contains(prompt, "BOUNDED_CANDIDATES"):
		candidateID, err := desiredStateProductSelectDeclarationCandidate(
			prompt, "Obsolete",
		)
		if err != nil {
			return "", assemblyline.WorkArtifactCandidateSelection, err
		}
		return candidateID, assemblyline.WorkArtifactCandidateSelection, nil
	case strings.Contains(prompt, "FOCUSED_ARTIFACT"):
		_, err := desiredStateProductPromptLine(prompt, "FOCUSED_ARTIFACT: ")
		if err != nil {
			return "", assemblyline.WorkArtifactHandling, err
		}
		return string(assemblyline.ArtifactMustBeAbsent), assemblyline.WorkArtifactHandling, nil
	case strings.Contains(prompt, "EXACT_SIGNATURE:\nfunc Added() int"):
		return "func Added() int { return 2 }", assemblyline.WorkFragmentGeneration, nil
	default:
		return "", "", fmt.Errorf("unexpected raw product vertical envelope")
	}
}

func desiredStateProductRequirementSource(prompt string) (string, error) {
	input, err := desiredStateProductRequirementProjection(prompt)
	if err != nil {
		return "", err
	}
	source := strings.TrimSpace(input.UserRequest)
	if source == "" {
		return "", fmt.Errorf("product vertical repository requirement source is empty")
	}
	return source, nil
}

type desiredStateProductRequirementInput struct {
	UserRequest   string
	AcceptedCount int
}

func desiredStateProductRequirementProjection(
	prompt string,
) (desiredStateProductRequirementInput, error) {
	const marker = "REPOSITORY REQUIREMENT INPUT:\n"
	index := strings.LastIndex(prompt, marker)
	if index < 0 {
		return desiredStateProductRequirementInput{}, fmt.Errorf(
			"product vertical repository requirement omitted semantic input",
		)
	}
	projection := strings.TrimSpace(prompt[index+len(marker):])
	const requestMarker = "IMMUTABLE USER REQUEST:\n"
	const workspaceMarker = "\nWORKSPACE STATE:\n"
	if !strings.HasPrefix(projection, requestMarker) {
		return desiredStateProductRequirementInput{}, fmt.Errorf(
			"product vertical repository requirement omitted immutable user request",
		)
	}
	requestAndRest := projection[len(requestMarker):]
	workspaceIndex := strings.Index(requestAndRest, workspaceMarker)
	if workspaceIndex < 0 {
		return desiredStateProductRequirementInput{}, fmt.Errorf(
			"product vertical repository requirement omitted workspace state",
		)
	}
	request := strings.TrimSpace(requestAndRest[:workspaceIndex])
	if request == "" {
		return desiredStateProductRequirementInput{}, fmt.Errorf(
			"product vertical repository requirement source is empty",
		)
	}
	acceptedCount := strings.Count(projection, "\nACCEPTED REQUIREMENT ")
	if acceptedCount == 0 && !strings.Contains(
		projection, "\nACCEPTED REQUIREMENTS:\n(none)",
	) {
		return desiredStateProductRequirementInput{}, fmt.Errorf(
			"product vertical repository requirement omitted accepted semantic set",
		)
	}
	return desiredStateProductRequirementInput{
		UserRequest: request, AcceptedCount: acceptedCount,
	}, nil
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
	tokenizer := []byte(`{"capabilities":["completion","vision","tools","thinking"],"model_info":{"general.architecture":"qwen35","tokenizer.ggml.model":"gpt2","tokenizer.ggml.pre":"qwen35","tokenizer.ggml.add_eos_token":false,"tokenizer.ggml.add_padding_token":false,"tokenizer.ggml.tokens":null,"tokenizer.ggml.token_type":null,"tokenizer.ggml.merges":null},"parameters":"temperature                    1\ntop_k                          20\ntop_p                          0.95\npresence_penalty               1.5","template":"{{ .Prompt }}"}`)
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
	if prepared.ProviderIdentityExpectation == nil {
		return llm.PreparedGeneration{}, fmt.Errorf(
			"ordinary product generation omitted its exact provider identity",
		)
	}
	selection, err := llm.ProviderIdentitySelectionForExpectation(
		*prepared.ProviderIdentityExpectation,
	)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	if selection.ProfilePolicy != "" {
		return llm.PreparedGeneration{}, fmt.Errorf(
			"ordinary product generation received non-strict profile policy %q",
			selection.ProfilePolicy,
		)
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
