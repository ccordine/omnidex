package omni

import (
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/secrets"
)

func cursorAPIKeyConfigured() bool {
	return strings.TrimSpace(secrets.Lookup("cursor_api_key")) != "" || strings.TrimSpace(os.Getenv("CURSOR_API_KEY")) != ""
}

func codexAPIKeyConfigured() bool { return strings.TrimSpace(secrets.CodexAPIKey()) != "" }

func CursorSDKEnabled() bool { return cursorAPIKeyConfigured() }

func CodexSDKEnabled() bool { return codexAPIKeyConfigured() }

func CursorSDKUnavailableReason() string {
	if CursorSDKEnabled() {
		return ""
	}
	return "Cursor API key is not configured (Admin → API secrets, or CURSOR_API_KEY in env)"
}

func CodexSDKUnavailableReason() string {
	if CodexSDKEnabled() {
		return ""
	}
	return "Codex API key is not configured (Admin → API secrets, or CODEX_API_KEY / OPENAI_API_KEY in env)"
}
