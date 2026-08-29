package worker

import (
	"strings"
	"testing"
)

func TestTypeScriptDeclarationLengthEmitsReviewWarningWithoutBecomingFailure(t *testing.T) {
	t.Parallel()

	if warning := directCodingTypeScriptDeclarationSizeWarning(
		directCodingTypeScriptDeclarationWarningBytes,
	); warning != "" {
		t.Fatalf("boundary warning=%q", warning)
	}
	warning := directCodingTypeScriptDeclarationSizeWarning(
		directCodingTypeScriptDeclarationWarningBytes + 40,
	)
	if !strings.Contains(warning, "declaration_size_warning") ||
		!strings.Contains(warning, "bytes=5160") ||
		!strings.Contains(warning, "threshold=5120") {
		t.Fatalf("warning=%q", warning)
	}

	rendered := renderDirectCodingWorkerEvent(typedWorkerEvent{
		State: typedWorkerCompleted, Kind: typedWorkerFragment,
		Subject: "feature.001", Model: "test-model", Warning: warning,
	})
	if !strings.Contains(rendered, "warning=declaration_size_warning") {
		t.Fatalf("rendered event omitted size warning: %s", rendered)
	}
	if strings.Contains(rendered, "error=") {
		t.Fatalf("advisory size warning became an error: %s", rendered)
	}
}
