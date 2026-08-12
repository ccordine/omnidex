package cognitionreplay

import (
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

const (
	ProviderIdentityEvidenceReplaySchemaV1   = "omnidex.replay-provider-identity-evidence.v1"
	ProviderRequestDispositionReplaySchemaV1 = "omnidex.replay-provider-request-disposition.v1"

	SourcePublicRunAuthority           = "public_run_authority"
	SourceProviderFailureAuthority     = "provider_activation_failure_authority"
	SourceBrainBootstrapFailureReceipt = "brain_bootstrap_failure_receipt"
	SourceProviderIdentityEvidence     = "provider_identity_evidence"
	SourceProviderIdentityRequestBody  = "provider_identity_request_body"
	SourceProviderIdentityResponseBody = "provider_identity_response_body"
)

type EvidenceBodyStorage string

const (
	EvidenceBodyEmpty  EvidenceBodyStorage = "empty"
	EvidenceBodySource EvidenceBodyStorage = "source"
)

type ProviderIdentityBodyBinding struct {
	SHA256    string              `json:"sha256"`
	ByteCount int                 `json:"byte_count"`
	Storage   EvidenceBodyStorage `json:"storage"`
	Source    *SourceRef          `json:"source,omitempty"`
}

type ProviderIdentityReplayOperation struct {
	Index              int                                      `json:"index"`
	Operation          llm.ProviderIdentityOperation            `json:"operation"`
	Method             string                                   `json:"method"`
	Endpoint           string                                   `json:"endpoint"`
	RequestDisposition llm.ProviderRequestDisposition           `json:"request_disposition"`
	Request            ProviderIdentityBodyBinding              `json:"request"`
	HTTPStatus         int                                      `json:"http_status"`
	Disposition        llm.ProviderIdentityOperationDisposition `json:"disposition"`
	ResponseComplete   bool                                     `json:"response_complete"`
	ContentEncoding    llm.ProviderContentEncodingEvidence      `json:"content_encoding"`
	Response           ProviderIdentityBodyBinding              `json:"response"`
}

type ProviderIdentityEvidenceReplay struct {
	Schema     string                            `json:"schema"`
	Ref        llm.ProviderIdentityEvidenceRef   `json:"ref"`
	Operations []ProviderIdentityReplayOperation `json:"operations"`
}

type ProviderRequestDispositionReplay struct {
	Schema             string                                   `json:"schema"`
	EvidenceID         string                                   `json:"evidence_id"`
	OperationIndex     int                                      `json:"operation_index"`
	Operation          llm.ProviderIdentityOperation            `json:"operation"`
	RequestDisposition llm.ProviderRequestDisposition           `json:"request_disposition"`
	Disposition        llm.ProviderIdentityOperationDisposition `json:"disposition"`
	HTTPStatus         int                                      `json:"http_status"`
	ResponseComplete   bool                                     `json:"response_complete"`
}

func (value ProviderIdentityEvidenceReplay) Validate() error {
	if value.Schema != ProviderIdentityEvidenceReplaySchemaV1 || value.Ref.Validate() != nil ||
		len(value.Operations) != 5 {
		return fmt.Errorf("replay provider identity evidence authority is invalid")
	}
	for index, operation := range value.Operations {
		if operation.Index != index ||
			validateProviderIdentityBodyBinding(operation.Request) != nil ||
			validateProviderIdentityBodyBinding(operation.Response) != nil {
			return fmt.Errorf("replay provider identity operation %d is invalid", index)
		}
	}
	return nil
}

func validateProviderIdentityBodyBinding(value ProviderIdentityBodyBinding) error {
	if !validDigest(value.SHA256) || value.ByteCount < 0 ||
		value.ByteCount > llm.MaxProviderIdentityComponentBytes+1 {
		return fmt.Errorf("replay provider identity body authority is invalid")
	}
	if value.ByteCount == 0 {
		if value.Storage != EvidenceBodyEmpty || value.Source != nil ||
			value.SHA256 != digestBytes(nil) {
			return fmt.Errorf("empty replay provider body is not explicit")
		}
		return nil
	}
	if value.Storage != EvidenceBodySource || value.Source == nil || value.Source.Validate() != nil {
		return fmt.Errorf("nonempty replay provider body lacks one source")
	}
	return nil
}

func (value ProviderRequestDispositionReplay) Validate() error {
	if value.Schema != ProviderRequestDispositionReplaySchemaV1 ||
		!validPrefixedDigest(value.EvidenceID, "provider_identity_") ||
		value.OperationIndex < 0 || value.OperationIndex > 4 {
		return fmt.Errorf("replay provider request disposition is invalid")
	}
	return nil
}

func providerBodySourceID(evidenceID string, index int, kind string) string {
	return fmt.Sprintf("%s:%d:%s", evidenceID, index, kind)
}
