package cognitionruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
)

const CancellationEvidenceSchemaV1 = "omnidex.cognition-cancellation-evidence.v1"

type CancellationCode string

const (
	CancellationPolicyFailure      CancellationCode = "policy_failure"
	CancellationRunBudgetExhausted CancellationCode = "run_budget_exhausted"
	CancellationProviderActivation CancellationCode = "provider_activation_failed"
	CancellationJobCanceled        CancellationCode = "job_canceled"
	CancellationGenerationRetired  CancellationCode = "generation_superseded"
)

type CancellationEvidence struct {
	Schema            string           `json:"schema"`
	ID                string           `json:"id"`
	Code              CancellationCode `json:"code"`
	SourceErrorSHA256 string           `json:"source_error_sha256"`
	PublicMessage     string           `json:"public_message"`
	SHA256            string           `json:"sha256"`
}

const providerActivationCancellationMessage = "Provider activation failed before cognition could resume."

func NewProviderActivationCancellationEvidence(
	failureRecordID string,
) (CancellationEvidence, error) {
	const prefix = "cognition_provider_failure_"
	digest := strings.TrimPrefix(failureRecordID, prefix)
	if failureRecordID != prefix+digest || !validSHA256(digest) {
		return CancellationEvidence{}, fmt.Errorf(
			"provider activation cancellation requires an exact failure record identity",
		)
	}
	value := CancellationEvidence{
		Schema: CancellationEvidenceSchemaV1, Code: CancellationProviderActivation,
		SourceErrorSHA256: digest, PublicMessage: providerActivationCancellationMessage,
	}
	value.SHA256 = cancellationEvidenceSHA(value)
	value.ID = "cognition_cancellation_evidence_" + value.SHA256
	return value, value.Validate()
}

type CancellationCommand struct {
	Binding          Binding                 `json:"binding"`
	ExpectedRevision cognition.WorldRevision `json:"expected_revision"`
	Code             CancellationCode        `json:"code"`
	SourceEvidence   CancellationEvidence    `json:"source_evidence"`
}

type CancellationSeal struct {
	Episode          cognition.EpisodeRef `json:"episode"`
	Code             CancellationCode     `json:"code"`
	SourceEvidenceID string               `json:"source_evidence_id"`
	TraceSHA256      string               `json:"trace_sha256"`
}

func NewCancellationEvidence(
	code CancellationCode,
	publicMessage string,
	source error,
) (CancellationEvidence, error) {
	if !registeredWorkerCancellationCode(code) {
		return CancellationEvidence{}, fmt.Errorf("worker cognition cancellation code %q is not registered", code)
	}
	if source == nil {
		return CancellationEvidence{}, fmt.Errorf("cognition cancellation requires exact source error evidence")
	}
	sourceDigest := sha256.Sum256([]byte(source.Error()))
	value := CancellationEvidence{
		Schema: CancellationEvidenceSchemaV1, Code: code,
		SourceErrorSHA256: hex.EncodeToString(sourceDigest[:]), PublicMessage: publicMessage,
	}
	value.SHA256 = cancellationEvidenceSHA(value)
	value.ID = "cognition_cancellation_evidence_" + value.SHA256
	if err := value.Validate(); err != nil {
		return CancellationEvidence{}, err
	}
	return value, nil
}

// NewLifecycleCancellationEvidence creates code-owned lifecycle evidence from
// an immutable lifecycle-operation digest. It cannot be submitted through the
// worker CancellationCommand authority.
func NewLifecycleCancellationEvidence(
	code CancellationCode,
	publicMessage string,
	lifecycleOperationSHA256 string,
) (CancellationEvidence, error) {
	if !registeredLifecycleCancellationCode(code) || !validSHA256(lifecycleOperationSHA256) {
		return CancellationEvidence{}, fmt.Errorf("lifecycle cognition cancellation authority is invalid")
	}
	value := CancellationEvidence{
		Schema: CancellationEvidenceSchemaV1, Code: code,
		SourceErrorSHA256: lifecycleOperationSHA256, PublicMessage: publicMessage,
	}
	value.SHA256 = cancellationEvidenceSHA(value)
	value.ID = "cognition_cancellation_evidence_" + value.SHA256
	if err := value.Validate(); err != nil {
		return CancellationEvidence{}, err
	}
	return value, nil
}

func (evidence CancellationEvidence) Validate() error {
	if evidence.Schema != CancellationEvidenceSchemaV1 || !registeredCancellationCode(evidence.Code) ||
		!validSHA256(evidence.SourceErrorSHA256) ||
		evidence.ID != "cognition_cancellation_evidence_"+evidence.SHA256 ||
		!validSHA256(evidence.SHA256) || evidence.SHA256 != cancellationEvidenceSHA(evidence) {
		return fmt.Errorf("cognition cancellation evidence identity is invalid")
	}
	if !utf8.ValidString(evidence.PublicMessage) || evidence.PublicMessage == "" ||
		evidence.PublicMessage != strings.TrimSpace(evidence.PublicMessage) ||
		strings.ContainsRune(evidence.PublicMessage, 0) ||
		len(evidence.PublicMessage) > cognition.MaxPublicOutcomeBytes {
		return fmt.Errorf("cognition cancellation public message must be exact bounded UTF-8")
	}
	return nil
}

func (command CancellationCommand) Validate() error {
	if err := command.Binding.Validate(); err != nil {
		return err
	}
	if err := command.ExpectedRevision.Validate(); err != nil ||
		command.ExpectedRevision.EpisodeID != command.Binding.Episode.ID {
		return fmt.Errorf("cognition cancellation revision does not bind the episode")
	}
	if err := command.SourceEvidence.Validate(); err != nil {
		return err
	}
	if !registeredWorkerCancellationCode(command.Code) || command.SourceEvidence.Code != command.Code {
		return fmt.Errorf("cognition cancellation code is not registered or differs from its evidence")
	}
	return nil
}

func (seal CancellationSeal) ValidateFor(command CancellationCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	if seal.Episode != command.Binding.Episode || seal.Code != command.Code ||
		seal.SourceEvidenceID != command.SourceEvidence.ID || !validSHA256(seal.TraceSHA256) {
		return fmt.Errorf("cognition cancellation seal does not bind the exact command")
	}
	return nil
}

func (seal CancellationSeal) Validate() error {
	if err := seal.Episode.Validate(); err != nil || !registeredCancellationCode(seal.Code) ||
		!strings.HasPrefix(seal.SourceEvidenceID, "cognition_cancellation_evidence_") ||
		len(seal.SourceEvidenceID) != len("cognition_cancellation_evidence_")+64 ||
		!validSHA256(strings.TrimPrefix(seal.SourceEvidenceID, "cognition_cancellation_evidence_")) ||
		!validSHA256(seal.TraceSHA256) {
		return fmt.Errorf("cognition cancellation seal is invalid")
	}
	return nil
}

func registeredCancellationCode(code CancellationCode) bool {
	return registeredWorkerCancellationCode(code) || registeredLifecycleCancellationCode(code)
}

func registeredWorkerCancellationCode(code CancellationCode) bool {
	return code == CancellationPolicyFailure || code == CancellationRunBudgetExhausted ||
		code == CancellationProviderActivation
}

func registeredLifecycleCancellationCode(code CancellationCode) bool {
	return code == CancellationJobCanceled || code == CancellationGenerationRetired
}

func cancellationEvidenceSHA(value CancellationEvidence) string {
	value.ID, value.SHA256 = "", ""
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
