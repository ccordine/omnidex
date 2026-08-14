package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/exactjson"
)

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
	installedModel, err := exactIdentitySelectedModel(installed.Models, selection.Model, false)
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
	runningModel, err := exactIdentitySelectedModel(running.Models, selection.Model, true)
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
	return deriveExactProviderModelProfile(response)
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

func exactIdentitySelectedModel(
	models []exactIdentityModel,
	name string,
	requireRunner bool,
) (exactIdentityModel, error) {
	found := make([]exactIdentityModel, 0, 1)
	for _, model := range models {
		if model.Name == name && (model.Model == "" || model.Model == name) {
			found = append(found, model)
		}
	}
	if len(found) != 1 || (requireRunner && found[0].ContextLength <= 0) {
		return exactIdentityModel{}, fmt.Errorf("provider model %q matched %d exact records", name, len(found))
	}
	return found[0], nil
}
