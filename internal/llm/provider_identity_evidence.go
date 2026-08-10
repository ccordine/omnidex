package llm

import "fmt"

const (
	ProviderIdentityEvidenceSchemaV1    = "omnidex.provider-identity-evidence.v1"
	ProviderIdentityEvidenceRefSchemaV1 = "omnidex.provider-identity-evidence-ref.v1"
	MaxProviderIdentityComponentBytes   = 4 * 1024 * 1024
	MaxProviderIdentityEvidenceBytes    = 7 * (MaxProviderIdentityComponentBytes + 1)
)

type ProviderIdentityOperation string

const (
	ProviderIdentityVersion   ProviderIdentityOperation = "version"
	ProviderIdentityInstalled ProviderIdentityOperation = "installed"
	ProviderIdentityTokenizer ProviderIdentityOperation = "tokenizer"
	ProviderIdentityPreload   ProviderIdentityOperation = "preload"
	ProviderIdentityRunner    ProviderIdentityOperation = "runner"
)

type ProviderIdentityOperationDisposition string

const (
	ProviderIdentityNotDispatched ProviderIdentityOperationDisposition = "not_dispatched"
	ProviderIdentitySucceeded     ProviderIdentityOperationDisposition = "succeeded"
	ProviderIdentityTransport     ProviderIdentityOperationDisposition = "transport_error"
	ProviderIdentityHTTPError     ProviderIdentityOperationDisposition = "http_error"
	ProviderIdentityBodyLimit     ProviderIdentityOperationDisposition = "body_limit"
	ProviderIdentityBodyReadError ProviderIdentityOperationDisposition = "body_read_error"
	ProviderIdentityInvalidJSON   ProviderIdentityOperationDisposition = "invalid_json"
)

type ProviderIdentityEvidenceRef struct {
	Schema string `json:"schema"`
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

type ProviderIdentityOperationEvidence struct {
	Operation            ProviderIdentityOperation            `json:"operation"`
	Method               string                               `json:"method"`
	Endpoint             string                               `json:"endpoint"`
	RequestDispatched    bool                                 `json:"request_dispatched"`
	RequestSHA256        string                               `json:"request_sha256"`
	RequestBytes         int                                  `json:"request_bytes"`
	Request              []byte                               `json:"-"`
	HTTPStatus           int                                  `json:"http_status"`
	Disposition          ProviderIdentityOperationDisposition `json:"disposition"`
	ResponseComplete     bool                                 `json:"response_complete"`
	ContentEncodingCount int                                  `json:"content_encoding_count"`
	ContentEncoding      string                               `json:"content_encoding"`
	ResponseUncompressed bool                                 `json:"response_uncompressed"`
	ResponseSHA256       string                               `json:"response_sha256"`
	ResponseBytes        int                                  `json:"response_bytes"`
	ResponseCapture      []byte                               `json:"-"`
}

type ProviderIdentityEvidence struct {
	Schema     string                              `json:"schema"`
	Ref        ProviderIdentityEvidenceRef         `json:"ref"`
	Operations []ProviderIdentityOperationEvidence `json:"operations"`
}

func (ref ProviderIdentityEvidenceRef) Validate() error {
	if ref.Schema != ProviderIdentityEvidenceRefSchemaV1 ||
		ref.ID != "provider_identity_"+ref.SHA256 ||
		!providerIdentityDigest.MatchString(ref.SHA256) ||
		ref.Bytes < 1 || ref.Bytes > MaxProviderIdentityEvidenceBytes {
		return fmt.Errorf("provider identity evidence reference is invalid")
	}
	return nil
}

func NewProviderIdentityOperationEvidence(
	operation ProviderIdentityOperation,
	method string,
	endpoint string,
	requestDispatched bool,
	request []byte,
	status int,
	disposition ProviderIdentityOperationDisposition,
	responseComplete bool,
	contentEncodingCount int,
	contentEncoding string,
	responseUncompressed bool,
	response []byte,
) (ProviderIdentityOperationEvidence, error) {
	value := ProviderIdentityOperationEvidence{
		Operation: operation, Method: method, Endpoint: endpoint,
		RequestDispatched: requestDispatched, Request: append([]byte(nil), request...),
		RequestSHA256: providerBodySHA256(request), RequestBytes: len(request),
		HTTPStatus: status, Disposition: disposition, ResponseComplete: responseComplete,
		ContentEncodingCount: contentEncodingCount, ContentEncoding: contentEncoding,
		ResponseUncompressed: responseUncompressed,
		ResponseCapture:      append([]byte(nil), response...),
		ResponseSHA256:       providerBodySHA256(response), ResponseBytes: len(response),
	}
	if err := value.Validate(); err != nil {
		return ProviderIdentityOperationEvidence{}, err
	}
	return value, nil
}

func NewProviderIdentityEvidence(
	operations []ProviderIdentityOperationEvidence,
) (ProviderIdentityEvidence, error) {
	value := ProviderIdentityEvidence{
		Schema:     ProviderIdentityEvidenceSchemaV1,
		Operations: cloneProviderIdentityOperations(operations),
	}
	manifest, err := providerIdentityEvidenceManifest(value)
	if err != nil {
		return ProviderIdentityEvidence{}, err
	}
	value.Ref = ProviderIdentityEvidenceRef{
		Schema: ProviderIdentityEvidenceRefSchemaV1,
		SHA256: providerBodySHA256(manifest), Bytes: providerIdentityEvidenceBytes(value.Operations),
	}
	value.Ref.ID = "provider_identity_" + value.Ref.SHA256
	if err := value.Validate(); err != nil {
		return ProviderIdentityEvidence{}, err
	}
	return value, nil
}

func NewSuccessfulProviderIdentityEvidence(
	versionResponse []byte,
	installedResponse []byte,
	tokenizerRequest []byte,
	tokenizerResponse []byte,
	preloadRequest []byte,
	preloadResponse []byte,
	runnerResponse []byte,
) (ProviderIdentityEvidence, error) {
	definitions := []struct {
		operation         ProviderIdentityOperation
		method, endpoint  string
		request, response []byte
	}{
		{ProviderIdentityVersion, "GET", "/api/version", nil, versionResponse},
		{ProviderIdentityInstalled, "GET", "/api/tags", nil, installedResponse},
		{ProviderIdentityTokenizer, "POST", "/api/show", tokenizerRequest, tokenizerResponse},
		{ProviderIdentityPreload, "POST", "/api/generate", preloadRequest, preloadResponse},
		{ProviderIdentityRunner, "GET", "/api/ps", nil, runnerResponse},
	}
	operations := make([]ProviderIdentityOperationEvidence, 0, len(definitions))
	for _, definition := range definitions {
		operation, err := NewProviderIdentityOperationEvidence(
			definition.operation, definition.method, definition.endpoint, true,
			definition.request, 200, ProviderIdentitySucceeded, true, 0, "", false,
			definition.response,
		)
		if err != nil {
			return ProviderIdentityEvidence{}, err
		}
		operations = append(operations, operation)
	}
	return NewProviderIdentityEvidence(operations)
}

func (evidence ProviderIdentityEvidence) Validate() error {
	if evidence.Schema != ProviderIdentityEvidenceSchemaV1 || len(evidence.Operations) != 5 ||
		evidence.Ref.Validate() != nil ||
		evidence.Ref.Bytes != providerIdentityEvidenceBytes(evidence.Operations) ||
		evidence.Ref.Bytes < 1 || evidence.Ref.Bytes > MaxProviderIdentityEvidenceBytes {
		return fmt.Errorf("provider identity evidence identity is invalid")
	}
	stopped := false
	for index, operation := range evidence.Operations {
		wantOperation, wantMethod, wantEndpoint := providerIdentityOperationAt(index)
		if operation.Operation != wantOperation || operation.Method != wantMethod ||
			operation.Endpoint != wantEndpoint || operation.Validate() != nil {
			return fmt.Errorf("provider identity operation %d is invalid", index)
		}
		if stopped && operation.Disposition != ProviderIdentityNotDispatched {
			return fmt.Errorf("provider identity evidence continued after an operation failure")
		}
		if !stopped && operation.Disposition == ProviderIdentityNotDispatched {
			return fmt.Errorf("provider identity evidence stopped without a dispatched failure")
		}
		if !stopped && operation.Disposition != ProviderIdentitySucceeded {
			stopped = true
		}
	}
	manifest, err := providerIdentityEvidenceManifest(evidence)
	if err != nil || providerBodySHA256(manifest) != evidence.Ref.SHA256 {
		return fmt.Errorf("provider identity evidence hash changed")
	}
	return nil
}

func (operation ProviderIdentityOperationEvidence) Validate() error {
	if operation.RequestBytes != len(operation.Request) ||
		operation.RequestBytes > MaxProviderIdentityComponentBytes ||
		operation.RequestSHA256 != providerBodySHA256(operation.Request) ||
		operation.ResponseBytes != len(operation.ResponseCapture) ||
		operation.ResponseBytes > MaxProviderIdentityComponentBytes+1 ||
		operation.ResponseSHA256 != providerBodySHA256(operation.ResponseCapture) {
		return fmt.Errorf("provider identity operation bytes differ from their identity")
	}
	if !validProviderContentEncoding(
		operation.ContentEncodingCount, operation.ContentEncoding,
	) {
		return fmt.Errorf("provider identity operation content encoding is invalid")
	}
	if operation.Disposition == ProviderIdentityNotDispatched {
		if operation.RequestDispatched || operation.HTTPStatus != 0 || operation.ResponseComplete ||
			operation.ResponseBytes != 0 || operation.ContentEncodingCount != 0 ||
			operation.ResponseUncompressed {
			return fmt.Errorf("undispatched provider identity operation claims a request")
		}
		return nil
	}
	if !operation.RequestDispatched {
		return fmt.Errorf("provider identity operation disposition lacks dispatch")
	}
	switch operation.Disposition {
	case ProviderIdentityTransport:
		if operation.HTTPStatus != 0 || operation.ResponseComplete || operation.ResponseBytes != 0 ||
			operation.ContentEncodingCount != 0 || operation.ContentEncoding != "" ||
			operation.ResponseUncompressed {
			return fmt.Errorf("provider identity transport failure claims a response")
		}
	case ProviderIdentityBodyLimit:
		if operation.HTTPStatus < 100 || operation.HTTPStatus > 599 || operation.ResponseComplete ||
			operation.ResponseBytes != MaxProviderIdentityComponentBytes+1 {
			return fmt.Errorf("provider identity body limit lacks its exact prefix")
		}
	case ProviderIdentityBodyReadError:
		if operation.HTTPStatus < 100 || operation.HTTPStatus > 599 || operation.ResponseComplete {
			return fmt.Errorf("provider identity body read failure claims completeness")
		}
	case ProviderIdentitySucceeded, ProviderIdentityHTTPError, ProviderIdentityInvalidJSON:
		if operation.HTTPStatus < 100 || operation.HTTPStatus > 599 || !operation.ResponseComplete ||
			operation.ResponseBytes > MaxProviderIdentityComponentBytes {
			return fmt.Errorf("complete provider identity operation is invalid")
		}
		if (operation.Disposition == ProviderIdentitySucceeded ||
			operation.Disposition == ProviderIdentityInvalidJSON) !=
			(operation.HTTPStatus >= 200 && operation.HTTPStatus < 300) {
			return fmt.Errorf("provider identity response status differs from disposition")
		}
		if operation.Disposition == ProviderIdentitySucceeded &&
			(operation.ResponseUncompressed || !ProviderContentEncodingIsIdentity(
				operation.ContentEncodingCount, operation.ContentEncoding,
			)) {
			return fmt.Errorf("successful provider identity response used transformed encoding")
		}
	default:
		return fmt.Errorf("provider identity operation disposition is not registered")
	}
	return nil
}

func (evidence ProviderIdentityEvidence) Successful() bool {
	if evidence.Validate() != nil {
		return false
	}
	for _, operation := range evidence.Operations {
		if operation.Disposition != ProviderIdentitySucceeded {
			return false
		}
	}
	return true
}

func providerIdentityOperationAt(index int) (ProviderIdentityOperation, string, string) {
	values := []struct {
		operation        ProviderIdentityOperation
		method, endpoint string
	}{
		{ProviderIdentityVersion, "GET", "/api/version"},
		{ProviderIdentityInstalled, "GET", "/api/tags"},
		{ProviderIdentityTokenizer, "POST", "/api/show"},
		{ProviderIdentityPreload, "POST", "/api/generate"},
		{ProviderIdentityRunner, "GET", "/api/ps"},
	}
	if index < 0 || index >= len(values) {
		return "", "", ""
	}
	return values[index].operation, values[index].method, values[index].endpoint
}

func cloneProviderIdentityOperations(values []ProviderIdentityOperationEvidence) []ProviderIdentityOperationEvidence {
	cloned := append([]ProviderIdentityOperationEvidence(nil), values...)
	for index := range cloned {
		cloned[index].Request = append([]byte(nil), cloned[index].Request...)
		cloned[index].ResponseCapture = append([]byte(nil), cloned[index].ResponseCapture...)
	}
	return cloned
}
