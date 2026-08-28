package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/exactjson"
)

// DeriveRoleplayCompletionContextLimit reads the architecture-owned native context
// field from one exact /api/show body and clamps it to the configured request
// ceiling. No model output participates in this decision.
func DeriveRoleplayCompletionContextLimit(showResponse []byte, requested int) (int, error) {
	if err := ValidateInferenceContextTokens(requested); err != nil {
		return 0, fmt.Errorf("roleplay completion configured context: %w", err)
	}
	if err := exactjson.ValidateCompatibleObject(
		showResponse, exactIdentityShowResponse{}, "roleplay completion context response",
	); err != nil {
		return 0, err
	}
	var response exactIdentityShowResponse
	if err := json.Unmarshal(showResponse, &response); err != nil {
		return 0, err
	}
	if err := validateRoleplayCompletionProviderMetadata(response); err != nil {
		return 0, err
	}
	architecture, err := exactTokenizerString(response.ModelInfo, "general.architecture")
	if err != nil {
		return 0, err
	}
	field := architecture + ".context_length"
	raw, exists := response.ModelInfo[field]
	if !exists {
		return 0, fmt.Errorf("roleplay completion provider model is missing exact context field %q", field)
	}
	native, err := strconv.Atoi(string(raw))
	if err != nil || strconv.Itoa(native) != string(raw) {
		return 0, fmt.Errorf("roleplay completion provider context field %q is not an exact integer", field)
	}
	if err := ValidateRoleplayCompletionContextTokens(native); err != nil {
		return 0, fmt.Errorf("roleplay completion provider context field %q: %w", field, err)
	}
	if native < requested {
		return native, nil
	}
	return requested, nil
}

type exactIdentityVersionResponse struct {
	Version string `json:"version"`
}
type exactIdentityModelsResponse struct {
	Models []exactIdentityModel `json:"models"`
}
type exactIdentityModel struct {
	Name          string                    `json:"name"`
	Model         string                    `json:"model"`
	Size          int64                     `json:"size"`
	Digest        string                    `json:"digest"`
	Details       exactIdentityModelDetails `json:"details"`
	ModifiedAt    time.Time                 `json:"modified_at,omitempty"`
	ExpiresAt     time.Time                 `json:"expires_at,omitempty"`
	SizeVRAM      int64                     `json:"size_vram,omitempty"`
	ContextLength int                       `json:"context_length,omitempty"`
}
type exactIdentityModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}
type exactIdentityShowRequest struct {
	Model   string `json:"model"`
	Verbose bool   `json:"verbose"`
}
type exactIdentityShowResponse struct {
	Capabilities []string                   `json:"capabilities"`
	ModelInfo    map[string]json.RawMessage `json:"model_info"`
	Parameters   string                     `json:"parameters"`
	Template     string                     `json:"template"`
}
type exactIdentityPreloadRequest struct {
	Model     string `json:"model"`
	Stream    bool   `json:"stream"`
	KeepAlive string `json:"keep_alive"`
	Options   struct {
		NumCtx int `json:"num_ctx"`
	} `json:"options"`
}

func ExactProviderTokenizerRequestBytes(selection ProviderIdentitySelection) ([]byte, error) {
	if err := selection.Validate(); err != nil {
		return nil, err
	}
	return exactjson.Canonical(exactIdentityShowRequest{selection.Model, false})
}

func ExactProviderPreloadRequestBytes(selection ProviderIdentitySelection) ([]byte, error) {
	if err := selection.Validate(); err != nil {
		return nil, err
	}
	request := exactIdentityPreloadRequest{
		Model: selection.Model, Stream: false, KeepAlive: "5m",
	}
	request.Options.NumCtx = selection.NativeContextLimit
	return exactjson.Canonical(request)
}

func DeriveExactProviderIdentityExpectation(
	evidence ProviderIdentityEvidence,
	selection ProviderIdentitySelection,
) (ProviderIdentityExpectation, error) {
	if err := evidence.ValidateRequests(selection); err != nil || !evidence.Successful() {
		return ProviderIdentityExpectation{}, fmt.Errorf("exact provider identity evidence is not complete")
	}
	operations := evidence.Operations
	var version exactIdentityVersionResponse
	if err := decodeExactIdentityJSON(operations[0].ResponseCapture, &version, "version"); err != nil {
		return ProviderIdentityExpectation{}, err
	}
	var installed exactIdentityModelsResponse
	if err := decodeExactIdentityJSON(operations[1].ResponseCapture, &installed, "installed models"); err != nil {
		return ProviderIdentityExpectation{}, err
	}
	installedModel, err := exactIdentityInstalledModel(installed.Models, selection.Model)
	if err != nil {
		return ProviderIdentityExpectation{}, err
	}
	modelProfile, err := validateExactTokenizerOperation(operations[2], selection)
	if err != nil {
		return ProviderIdentityExpectation{}, err
	}
	if err := validateExactPreloadOperation(operations[3], selection); err != nil {
		return ProviderIdentityExpectation{}, err
	}
	var running exactIdentityModelsResponse
	if err := decodeExactIdentityJSON(operations[4].ResponseCapture, &running, "running models"); err != nil {
		return ProviderIdentityExpectation{}, err
	}
	runningModel, err := exactIdentityRunningModel(running.Models, installedModel)
	if err != nil {
		return ProviderIdentityExpectation{}, err
	}
	if runningModel.Digest != installedModel.Digest ||
		runningModel.Details.QuantizationLevel != installedModel.Details.QuantizationLevel ||
		runningModel.ContextLength != selection.NativeContextLimit {
		return ProviderIdentityExpectation{}, fmt.Errorf("running provider model differs from its installed identity")
	}
	if version.Version != strings.TrimSpace(version.Version) {
		return ProviderIdentityExpectation{}, fmt.Errorf("provider backend version is not exact text")
	}
	expected := ProviderIdentityExpectation{
		Backend: ExactPreparedProviderBackend, BackendVersion: version.Version,
		Model: selection.Model, Digest: installedModel.Digest,
		Quantization:       installedModel.Details.QuantizationLevel,
		NativeContextLimit: selection.NativeContextLimit,
		TokenizerProfile:   modelProfile.tokenizerProfile,
	}
	if err := ValidateExactPreparedProviderExpectation(expected); err != nil {
		return ProviderIdentityExpectation{}, err
	}
	return expected, nil
}

func validateExactTokenizerOperation(
	operation ProviderIdentityOperationEvidence,
	selection ProviderIdentitySelection,
) (exactProviderModelProfile, error) {
	wantRequest, err := ExactProviderTokenizerRequestBytes(selection)
	if err != nil || !bytes.Equal(operation.Request, wantRequest) {
		return exactProviderModelProfile{}, fmt.Errorf("exact tokenizer observation request changed")
	}
	if err := exactjson.ValidateCompatibleObject(
		operation.ResponseCapture, exactIdentityShowResponse{}, "exact tokenizer response",
	); err != nil {
		return exactProviderModelProfile{}, err
	}
	var response exactIdentityShowResponse
	if err := json.Unmarshal(operation.ResponseCapture, &response); err != nil {
		return exactProviderModelProfile{}, err
	}
	if selection.ProfilePolicy != "" {
		return deriveStructurallyAttestedRoleplayProviderModelProfile(
			response, selection.ProfilePolicy,
		)
	}
	return deriveExactProviderModelProfile(response)
}

func deriveStructurallyAttestedRoleplayProviderModelProfile(
	response exactIdentityShowResponse,
	policy ProviderIdentityProfilePolicy,
) (exactProviderModelProfile, error) {
	if err := policy.Validate(); err != nil || policy == "" {
		return exactProviderModelProfile{}, fmt.Errorf("roleplay completion provider policy is invalid")
	}
	if err := validateRoleplayCompletionProviderMetadata(response); err != nil {
		return exactProviderModelProfile{}, err
	}
	profile, err := deriveExactProviderModelProfile(response)
	if err != nil {
		return exactProviderModelProfile{}, fmt.Errorf(
			"roleplay completion provider lacks a structurally attested exact profile: %w",
			err,
		)
	}
	return profile, nil
}

func validateRoleplayCompletionProviderMetadata(response exactIdentityShowResponse) error {
	architecture, err := exactTokenizerString(response.ModelInfo, "general.architecture")
	if err != nil {
		return err
	}
	tokenizerModel, err := exactTokenizerString(response.ModelInfo, "tokenizer.ggml.model")
	if err != nil {
		return err
	}
	if _, err := exactTokenizerOptionalString(response.ModelInfo, "tokenizer.ggml.pre"); err != nil {
		return err
	}
	if !providerIdentityText(architecture, 256) || !providerIdentityText(tokenizerModel, 256) {
		return fmt.Errorf("roleplay completion provider tokenizer identity is not bounded text")
	}
	seen := make(map[string]struct{}, len(response.Capabilities))
	hasCompletion := false
	if len(response.Capabilities) == 0 || len(response.Capabilities) > 32 {
		return fmt.Errorf("roleplay completion provider capabilities are outside bounds")
	}
	for _, capability := range response.Capabilities {
		if !providerIdentityText(capability, 64) {
			return fmt.Errorf("roleplay completion provider capability is not bounded text")
		}
		if _, exists := seen[capability]; exists {
			return fmt.Errorf("roleplay completion provider capability %q is duplicated", capability)
		}
		seen[capability] = struct{}{}
		hasCompletion = hasCompletion || capability == "completion"
	}
	if !hasCompletion {
		return fmt.Errorf("roleplay completion provider does not advertise completion capability")
	}
	return nil
}

func validateExactPreloadOperation(
	operation ProviderIdentityOperationEvidence,
	selection ProviderIdentitySelection,
) error {
	wantRequest, err := ExactProviderPreloadRequestBytes(selection)
	if err != nil || !bytes.Equal(operation.Request, wantRequest) {
		return fmt.Errorf("exact provider preload request changed")
	}
	var request exactIdentityPreloadRequest
	if err := decodeExactIdentityJSON(operation.Request, &request, "preload request"); err != nil {
		return err
	}
	if request.Model != selection.Model || request.Stream || request.KeepAlive != "5m" ||
		request.Options.NumCtx != selection.NativeContextLimit {
		return fmt.Errorf("exact provider preload request changed")
	}
	if err := exactjson.ValidateUniqueObject(operation.ResponseCapture, "preload response"); err != nil {
		return err
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(operation.ResponseCapture, &response); err != nil {
		return err
	}
	var done bool
	if raw, exists := response["done"]; !exists || json.Unmarshal(raw, &done) != nil || !done {
		return fmt.Errorf("exact provider preload did not complete")
	}
	return nil
}

func decodeExactIdentityJSON(raw []byte, target any, subject string) error {
	if err := exactjson.ValidateObject(raw, target, "exact provider "+subject); err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func exactIdentityInstalledModel(models []exactIdentityModel, name string) (exactIdentityModel, error) {
	found := make([]exactIdentityModel, 0, 1)
	for _, model := range models {
		if model.Name == name && (model.Model == "" || model.Model == name) {
			found = append(found, model)
		}
	}
	if len(found) != 1 {
		return exactIdentityModel{}, fmt.Errorf("provider model %q matched %d exact records", name, len(found))
	}
	return found[0], nil
}

func exactIdentityRunningModel(
	models []exactIdentityModel,
	installed exactIdentityModel,
) (exactIdentityModel, error) {
	if !providerIdentityDigest.MatchString(installed.Digest) ||
		!providerIdentityText(installed.Details.QuantizationLevel, 256) {
		return exactIdentityModel{}, fmt.Errorf("installed provider model identity is invalid")
	}
	found := make([]exactIdentityModel, 0, 1)
	for _, model := range models {
		if model.Digest == installed.Digest &&
			model.Details.QuantizationLevel == installed.Details.QuantizationLevel &&
			providerIdentityText(model.Name, 256) &&
			(model.Model == "" || model.Model == model.Name) {
			found = append(found, model)
		}
	}
	if len(found) != 1 || found[0].ContextLength <= 0 {
		return exactIdentityModel{}, fmt.Errorf(
			"running provider identity for model %q matched %d exact records",
			installed.Name,
			len(found),
		)
	}
	return found[0], nil
}
