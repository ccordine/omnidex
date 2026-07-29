package omni

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestExternalAgentTimeoutRejectsInvalidConfiguration(t *testing.T) {
	const key = "OMNI_TEST_EXTERNAL_TIMEOUT"
	for _, value := range []string{"later", "0s", "-1s"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(key, value)
			if _, err := externalAgentTimeout(key, time.Minute); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("invalid timeout %q error=%v", value, err)
			}
		})
	}
	os.Unsetenv(key)
	got, err := externalAgentTimeout(key, time.Minute)
	if err != nil || got != time.Minute {
		t.Fatalf("default timeout=%s error=%v", got, err)
	}
}

func TestExternalSDKConfigurationHasNoEnableDisableGate(t *testing.T) {
	for _, path := range []string{"external_sdk_enable.go", "cursor_sdk_agent.go", "codex_sdk_agent.go"} {
		blob, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(blob)
		for _, forbidden := range []string{"OMNI_ENABLE_CURSOR_ARCHITECT", "OMNI_DISABLE_CURSOR_ARCHITECT", "OMNI_ENABLE_CODEX_ARCHITECT", "OMNI_DISABLE_CODEX_ARCHITECT", "envBoolOrDefault"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s retained obsolete gate %q", path, forbidden)
			}
		}
	}
}
