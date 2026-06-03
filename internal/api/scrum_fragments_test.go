package api

import (
	"strings"
	"testing"
)

func TestRenderRecyclrTemplateHTMLRequiresTarget(t *testing.T) {
	assertPanicsWith(t, "recyclr template target is required", func() {
		renderRecyclrTemplateHTML(" ", "<p>body</p>", "innerHTML")
	})
}

func TestRenderRecyclrTemplateHTMLRejectsUnsupportedLocation(t *testing.T) {
	assertPanicsWith(t, `unsupported recyclr template location: "append"`, func() {
		renderRecyclrTemplateHTML("timeline", "<p>body</p>", "append")
	})
}

func TestRenderRecyclrTemplateHTMLEscapesAttributes(t *testing.T) {
	html := renderRecyclrTemplateHTML(`bad"target`, "<p>body</p>", "innerHTML")
	if !strings.Contains(html, `data-recyclr-target="bad&#34;target"`) {
		t.Fatalf("expected escaped target attribute, got %q", html)
	}
}

func assertPanicsWith(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		got := recovered.(string)
		if !strings.Contains(got, want) {
			t.Fatalf("expected panic containing %q, got %q", want, got)
		}
	}()
	fn()
}
