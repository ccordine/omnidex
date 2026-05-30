package omni

import (
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/secrets"
)

func cursorAPIKeyConfigured() bool {
	if strings.TrimSpace(secrets.Lookup("cursor_api_key")) != "" {
		return true
	}
	return strings.TrimSpace(os.Getenv("CURSOR_API_KEY")) != ""
}

func codexAPIKeyConfigured() bool {
	return strings.TrimSpace(secrets.CodexAPIKey()) != ""
}

// CursorSDKEnabled reports whether Cursor can run. Explicit card/project/workspace
// selection only requires a configured API key — no OMNI_ENABLE_CURSOR_ARCHITECT gate.
func CursorSDKEnabled(explicitRequest bool) bool {
	if explicitRequest {
		return cursorAPIKeyConfigured()
	}
	if envBoolOrDefault("OMNI_DISABLE_CURSOR_ARCHITECT", false) {
		return false
	}
	if envBoolOrDefault("OMNI_ENABLE_CURSOR_ARCHITECT", false) {
		return true
	}
	return cursorAPIKeyConfigured()
}

// CodexSDKEnabled reports whether Codex can run. Explicit selection only requires a key.
func CodexSDKEnabled(explicitRequest bool) bool {
	if explicitRequest {
		return codexAPIKeyConfigured()
	}
	if envBoolOrDefault("OMNI_DISABLE_CODEX_ARCHITECT", false) {
		return false
	}
	if envBoolOrDefault("OMNI_ENABLE_CODEX_ARCHITECT", false) {
		return true
	}
	return codexAPIKeyConfigured()
}

func CursorSDKUnavailableReason(explicitRequest bool) string {
	if CursorSDKEnabled(explicitRequest) {
		return ""
	}
	if !explicitRequest && envBoolOrDefault("OMNI_DISABLE_CURSOR_ARCHITECT", false) {
		return "Cursor SDK is disabled (OMNI_DISABLE_CURSOR_ARCHITECT=true)"
	}
	return "Cursor API key is not configured (Admin → API secrets, or CURSOR_API_KEY in env)"
}

func CodexSDKUnavailableReason(explicitRequest bool) string {
	if CodexSDKEnabled(explicitRequest) {
		return ""
	}
	if !explicitRequest && envBoolOrDefault("OMNI_DISABLE_CODEX_ARCHITECT", false) {
		return "Codex SDK is disabled (OMNI_DISABLE_CODEX_ARCHITECT=true)"
	}
	return "Codex API key is not configured (Admin → API secrets, or CODEX_API_KEY / OPENAI_API_KEY in env)"
}
