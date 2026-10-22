package googleai

import "testing"

func TestEmbeddingWaitsForCallerCancellation(t *testing.T) {
	client := NewEmbedding("https://api.example.test", "key", "embedding-model")
	if client.httpClient.Timeout != 0 {
		t.Fatal("provider imposes a wall-clock deadline")
	}
}
