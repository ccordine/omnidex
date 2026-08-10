package ollama

import (
	"net/http"
	"net/http/httptrace"
	"sync"

	"github.com/gryph/omnidex/internal/llm"
)

const maxExactProviderResponseHeaderBytes = 64 * 1024

func (c *Client) doExactProviderRequest(
	request *http.Request,
) (*http.Response, llm.ProviderRequestDisposition, error) {
	request.Header.Set("Accept-Encoding", "identity")
	var write struct {
		sync.Mutex
		observed bool
		err      error
	}
	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		write.Lock()
		write.observed = true
		write.err = info.Err
		write.Unlock()
	}}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	client := *c.httpClient
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	response, err := client.Do(request)
	if response != nil {
		return response, llm.ProviderRequestDispatched, err
	}
	write.Lock()
	defer write.Unlock()
	if !write.observed {
		return nil, llm.ProviderRequestNotDispatched, err
	}
	if write.err != nil {
		return nil, llm.ProviderRequestWriteIndeterminate, err
	}
	return nil, llm.ProviderRequestDispatched, err
}

func exactProviderContentEncoding(response *http.Response) bool {
	return exactProviderContentEncodingEvidence(response).IsIdentity()
}

func exactProviderContentEncodingEvidence(response *http.Response) llm.ProviderContentEncodingEvidence {
	return llm.NewProviderContentEncodingEvidence(
		response.Header.Values("Content-Encoding"), response.Uncompressed,
	)
}
