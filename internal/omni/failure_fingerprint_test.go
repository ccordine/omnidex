package omni

import (
	"bytes"
	"strings"
	"testing"
)

func TestClassifyFailureDetectsKnownKinds(t *testing.T) {
	cases := map[string]string{
		"sh: webpack: command not found":                             "missing_command",
		"listen tcp 127.0.0.1:3000: bind: address already in use":    "port_in_use",
		"npm error 404 Not Found - GET https://registry.npmjs.org/x": "dependency_unavailable",
		"panic: open /root/file: permission denied":                  "permission_denied",
	}
	for input, want := range cases {
		if got := ClassifyFailure(input).Kind; got != want {
			t.Fatalf("ClassifyFailure(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestOmniFingerprintCommandReadsStdin(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader("curl: (6) Could not resolve host: example.invalid"), &out, &errOut)
	err := app.Run([]string{"fingerprint"})
	if err != nil {
		t.Fatalf("fingerprint failed: %v\nstderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "kind=network_failure") {
		t.Fatalf("fingerprint output = %q", out.String())
	}
}
