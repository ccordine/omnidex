package llm

import "github.com/gryph/omnidex/internal/exactjson"

func providerIdentityEvidenceManifest(evidence ProviderIdentityEvidence) ([]byte, error) {
	type operationManifest struct {
		Operation         ProviderIdentityOperation            `json:"operation"`
		Method            string                               `json:"method"`
		Endpoint          string                               `json:"endpoint"`
		RequestDispatched bool                                 `json:"request_dispatched"`
		RequestSHA256     string                               `json:"request_sha256"`
		RequestBytes      int                                  `json:"request_bytes"`
		HTTPStatus        int                                  `json:"http_status"`
		Disposition       ProviderIdentityOperationDisposition `json:"disposition"`
		ResponseComplete  bool                                 `json:"response_complete"`
		ContentEncoding   ProviderContentEncodingEvidence      `json:"content_encoding"`
		ResponseSHA256    string                               `json:"response_sha256"`
		ResponseBytes     int                                  `json:"response_bytes"`
	}
	operations := make([]operationManifest, len(evidence.Operations))
	for index, value := range evidence.Operations {
		operations[index] = operationManifest{
			value.Operation, value.Method, value.Endpoint, value.RequestDispatched,
			value.RequestSHA256, value.RequestBytes, value.HTTPStatus,
			value.Disposition, value.ResponseComplete, value.ContentEncoding,
			value.ResponseSHA256,
			value.ResponseBytes,
		}
	}
	return exactjson.Canonical(struct {
		Schema     string              `json:"schema"`
		Operations []operationManifest `json:"operations"`
	}{ProviderIdentityEvidenceSchemaV1, operations})
}

func providerIdentityEvidenceBytes(operations []ProviderIdentityOperationEvidence) int {
	total := 0
	for _, operation := range operations {
		total += len(operation.Request) + len(operation.ResponseCapture)
	}
	return total
}
