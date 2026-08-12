package ollama

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestChatPropagatesExactRequestWriteDisposition(t *testing.T) {
	transportFailure := errors.New("injected chat transport failure")
	writeFailure := errors.New("injected partial request write")
	for _, test := range []struct {
		name   string
		notify bool
		write  error
		want   llm.ProviderRequestDisposition
	}{
		{"prewrite", false, nil, llm.ProviderRequestNotDispatched},
		{"partial_write", true, writeFailure, llm.ProviderRequestWriteIndeterminate},
		{"full_write", true, nil, llm.ProviderRequestDispatched},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := New("http://ollama.test", "qwen", "embed", time.Second, 32768)
			client.httpClient.Transport = exactRoundTripFunc(func(request *http.Request) (
				*http.Response, error,
			) {
				if test.notify {
					notifyExactRequestWrite(request, test.write)
				}
				return nil, transportFailure
			})
			result, err := client.chatResponse(
				context.Background(), "qwen", "system", "user", 0, 32768, "", nil, false, nil,
			)
			if !errors.Is(err, transportFailure) || result.ProviderRequestDisposition != test.want {
				t.Fatalf("chat request disposition=%q error=%v, want %q/%v",
					result.ProviderRequestDisposition, err, test.want, transportFailure)
			}
		})
	}
}
