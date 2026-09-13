package assemblyline

import (
	"strings"
	"testing"
)

func TestResultValueKindReturnsOnlyOneBoundedTypeChoice(t *testing.T) {
	for _, fixture := range []struct {
		behavior, response string
		want               ApplicationResultValueKind
	}{
		{"Print the product of two integer arguments.", "B", ApplicationResultInteger},
		{"Print the supplied text unchanged.", "A", ApplicationResultText},
	} {
		input := ApplicationResultValueKindInput{Requirement: fixture.behavior}
		job, err := NewApplicationResultValueKindJob(input)
		if err != nil {
			t.Fatal(err)
		}
		prompt, err := RenderPortableJob(job)
		if err != nil || len(prompt) > 1024 || !strings.Contains(prompt, fixture.behavior) {
			t.Fatalf("prompt=%q error=%v", prompt, err)
		}
		for _, forbidden := range []string{"TaskInput", "TaskResult", "func ", "workspace", "signature", "JSON"} {
			if strings.Contains(prompt, forbidden) {
				t.Fatalf("value kind leaked %q", forbidden)
			}
		}
		kind, err := DecodeApplicationResultValueKind(input, fixture.response)
		if err != nil || kind != fixture.want {
			t.Fatalf("kind=%s error=%v", kind, err)
		}
		if _, err := DecodeApplicationResultValueKind(input, `{"kind":"integer"}`); err == nil {
			t.Fatal("accepted a model-authored record")
		}
	}
}
