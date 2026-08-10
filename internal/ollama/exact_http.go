package ollama

import (
	"net/http"

	"github.com/gryph/omnidex/internal/llm"
)

const maxExactProviderResponseHeaderBytes = 64 * 1024

func (c *Client) doExactProviderRequest(request *http.Request) (*http.Response, error) {
	request.Header.Set("Accept-Encoding", "identity")
	client := *c.httpClient
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client.Do(request)
}

func exactProviderContentEncoding(response *http.Response) bool {
	count, encoding, uncompressed := exactProviderContentEncodingEvidence(response)
	return !uncompressed && llm.ProviderContentEncodingIsIdentity(count, encoding)
}

func exactProviderContentEncodingEvidence(response *http.Response) (int, string, bool) {
	values := response.Header.Values("Content-Encoding")
	count, evidence, ok := llm.EncodeProviderContentEncodingEvidence(values)
	if !ok {
		return llm.MaxProviderContentEncodingValues + 1, "", response.Uncompressed
	}
	return count, evidence, response.Uncompressed
}
