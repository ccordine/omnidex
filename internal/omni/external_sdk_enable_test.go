package omni

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/secrets"
)

func TestCursorSDKEnabledWithDBKeyOnly(t *testing.T) {
	t.Setenv("OMNI_ENABLE_CURSOR_ARCHITECT", "")
	t.Setenv("CURSOR_API_KEY", "")
	t.Setenv("OMNI_DISABLE_CURSOR_ARCHITECT", "")

	secrets.SetGlobal(secrets.NewResolver(secretsStoreStub{
		values: map[string]string{"cursor_api_key": "test-key"},
	}))
	t.Cleanup(func() { secrets.SetGlobal(nil) })

	if !CursorSDKEnabled(false) {
		t.Fatal("expected cursor enabled when DB key is configured")
	}
	if NewCursorSDKArchitectAgent(true) == nil {
		t.Fatal("expected cursor agent when DB key is configured")
	}
}

func TestCursorSDKDisabledWithoutKeyOrFlag(t *testing.T) {
	t.Setenv("OMNI_ENABLE_CURSOR_ARCHITECT", "")
	t.Setenv("CURSOR_API_KEY", "")
	secrets.SetGlobal(nil)

	if CursorSDKEnabled(false) {
		t.Fatal("expected cursor disabled without key or enable flag")
	}
}

type secretsStoreStub struct {
	values map[string]string
}

func (s secretsStoreStub) GetAPISecrets(context.Context) (map[string]string, error) {
	return s.values, nil
}
