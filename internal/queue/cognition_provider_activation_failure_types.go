package queue

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

const MaxCognitionProviderActivationFailurePageSize = 16

type CognitionProviderActivationFailureRecord struct {
	RecordNumber        int64
	RecordID            string
	Kind                string
	EpisodeID           cognition.EpisodeID
	Actor               cognition.AttemptRef
	ReceiptSHA256       string
	AuthoritySHA256     string
	ReceiptJSON         []byte
	AuthorityJSON       []byte
	CreatedAt           time.Time
	Bootstrap           *cognitionpolicy.BrainBootstrapFailureReceipt
	Process             *cognitionpolicy.ProviderProcessFailureReceipt
	Evidence            CognitionProviderIdentityEvidenceManifest
	SuccessfulBootstrap *cognitionpolicy.AttestedBrain
	BootstrapEvidence   *CognitionProviderIdentityEvidenceManifest
}

type CognitionProviderActivationFailurePageRequest struct {
	Authority         model.StepAttemptAuthority
	EpisodeID         cognition.EpisodeID
	AfterRecordNumber int64
	Limit             int
}

type CognitionProviderActivationFailurePage struct {
	TotalRecords     int64
	NextRecordNumber int64
	Records          []CognitionProviderActivationFailureRecord
}

type CognitionProviderActivationFailureBodyRequest struct {
	Authority      model.StepAttemptAuthority
	EpisodeID      cognition.EpisodeID
	RecordID       string
	EvidenceID     string
	OperationIndex int
	Kind           CognitionProviderIdentityBodyKind
	Offset         int
	Limit          int
}

func (request CognitionProviderActivationFailurePageRequest) validate() error {
	if validateStepAttemptAuthority(request.Authority) != nil ||
		cognitionEpisodeIdentityValid(request.EpisodeID) != nil ||
		request.AfterRecordNumber < 0 || request.Limit < 1 ||
		request.Limit > MaxCognitionProviderActivationFailurePageSize {
		return fmt.Errorf("provider activation failure page request is invalid")
	}
	return nil
}

func (request CognitionProviderActivationFailureBodyRequest) validate() error {
	if validateStepAttemptAuthority(request.Authority) != nil ||
		cognitionEpisodeIdentityValid(request.EpisodeID) != nil ||
		!cognitionDigestIdentity(request.RecordID, "cognition_provider_failure_") ||
		!cognitionDigestIdentity(request.EvidenceID, "provider_identity_") ||
		request.OperationIndex < 0 || request.OperationIndex > 4 || request.Offset < 0 ||
		request.Limit < 1 || request.Limit > MaxCognitionPolicyEvidencePageBytes ||
		(request.Kind != CognitionProviderIdentityRequestBody &&
			request.Kind != CognitionProviderIdentityResponseBody) {
		return fmt.Errorf("provider activation failure body request is invalid")
	}
	return nil
}

func cognitionDigestIdentity(value string, prefix string) bool {
	return len(value) == len(prefix)+64 && value[:len(prefix)] == prefix &&
		cognitionDigestPattern.MatchString(value[len(prefix):])
}

func providerFailureEvidenceManifest(
	evidence llm.ProviderIdentityEvidence,
) CognitionProviderIdentityEvidenceManifest {
	manifest := CognitionProviderIdentityEvidenceManifest{
		Ref:        evidence.Ref,
		Operations: make([]CognitionProviderIdentityOperationMetadata, len(evidence.Operations)),
	}
	for index, operation := range evidence.Operations {
		manifest.Operations[index] = CognitionProviderIdentityOperationMetadata{
			Index: index, Operation: operation.Operation, Method: operation.Method,
			Endpoint: operation.Endpoint, RequestDisposition: operation.RequestDisposition,
			RequestSHA256: operation.RequestSHA256, RequestBytes: operation.RequestBytes,
			HTTPStatus: operation.HTTPStatus, Disposition: operation.Disposition,
			ResponseComplete: operation.ResponseComplete,
			ContentEncoding:  operation.ContentEncoding,
			ResponseSHA256:   operation.ResponseSHA256, ResponseBytes: operation.ResponseBytes,
		}
	}
	return manifest
}
