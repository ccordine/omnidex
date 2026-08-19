package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/queue"
)

func TestDelegatedDatabaseEvidenceRequiresLocalInferenceProvider(t *testing.T) {
	t.Parallel()
	if err := validateDelegatedDatabaseInferenceProvider("ollama"); err != nil {
		t.Fatalf("local provider rejected: %v", err)
	}
	for _, provider := range []string{"", "openai", "anthropic", "ollama "} {
		if err := validateDelegatedDatabaseInferenceProvider(provider); err == nil {
			t.Fatalf("non-local or inexact provider %q was accepted", provider)
		}
	}
}

func TestDelegatedDatabaseCredentialIsBoundToExactAuthorityURL(t *testing.T) {
	credentialEnv := "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN"
	urlEnv := "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_URL"
	t.Setenv(credentialEnv, "outbound-secret")
	t.Setenv(urlEnv, "https://different.internal")
	record := queue.DataSourceRecord{
		ExecutionMode: datasource.ExecutionModeDelegated,
		AuthorityURL:  "https://application.internal", CredentialEnv: credentialEnv,
	}
	if _, err := loadDelegatedDatabaseCredential(record); err == nil {
		t.Fatal("credential bound to a different authority URL was accepted")
	}
	t.Setenv(urlEnv, record.AuthorityURL)
	token, err := loadDelegatedDatabaseCredential(record)
	if err != nil {
		t.Fatal(err)
	}
	if token != "outbound-secret" {
		t.Fatalf("credential=%q", token)
	}
}
