package openai

import "testing"

func TestEmbeddingConstructorsHaveNoRequestDeadline(t *testing.T) {
	constructors := map[string]func() (*Client, error){
		"openai": func() (*Client, error) {
			return NewEmbedding("https://api.example.test/v1", "key", "embedding-model", "", "")
		},
		"compatible": func() (*Client, error) {
			return NewCompatibleEmbedding("compatible", "COMPATIBLE_API_KEY", "https://api.example.test/v1", "key", "embedding-model", "", "")
		},
		"azure": func() (*Client, error) {
			return NewAzureAIEmbedding("https://resource.openai.azure.com", "key", "embedding-model", "2024-10-21", "azure_openai")
		},
	}
	for name, construct := range constructors {
		t.Run(name, func(t *testing.T) {
			client, err := construct()
			if err != nil {
				t.Fatal(err)
			}
			if client.httpClient.Timeout != 0 {
				t.Fatal("provider imposes a wall-clock deadline")
			}
		})
	}
}
