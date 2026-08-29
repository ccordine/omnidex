package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/operation"
)

func TestResolvedComposeConfigHashesRequireExactCanonicalServiceAuthority(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name     string
		services []string
	}{
		{name: "web and data", services: []string{"api", "database"}},
		{name: "gateway and workers", services: []string{"cache", "gateway", "worker"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var lines []string
			for index, service := range testCase.services {
				lines = append(lines, service+" "+strings.Repeat(string(rune('a'+index)), 64))
			}
			got, err := parseDirectCodingResolvedConfigHashes(operation.Result{Output: map[string]any{
				"stdout": strings.Join(lines, "\n") + "\n", "stderr": "", "stdout_truncated": false,
			}}, testCase.services)
			if err != nil || len(got) != len(testCase.services) {
				t.Fatalf("resolved hashes=%+v err=%v", got, err)
			}
			for index, service := range got {
				if service.Service != testCase.services[index] || len(service.SHA256) != 64 {
					t.Fatalf("service %d=%+v", index, service)
				}
			}
		})
	}
}

func TestResolvedComposeConfigHashesRejectNonCanonicalOrDifferentServices(t *testing.T) {
	t.Parallel()
	validHash := strings.Repeat("a", 64)
	for name, stdout := range map[string]string{
		"different service": "worker " + validHash + "\n",
		"uppercase hash":    "api " + strings.Repeat("A", 64) + "\n",
		"extra spacing":     "api  " + validHash + "\n",
		"carriage return":   "api " + validHash + "\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseDirectCodingResolvedConfigHashes(operation.Result{Output: map[string]any{
				"stdout": stdout, "stderr": "", "stdout_truncated": false,
			}}, []string{"api"})
			if err == nil {
				t.Fatalf("accepted noncanonical output %q", stdout)
			}
		})
	}
}
