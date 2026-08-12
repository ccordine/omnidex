package codingobjective

import (
	"strings"
	"testing"
)

func TestGoTestPredicateAcceptsOnlyOrdinaryFailureExit(t *testing.T) {
	outcome, err := classifyGoTestExit(1, "one test failed")
	if err != nil || outcome.passed || outcome.diagnostic != "one test failed" {
		t.Fatalf("ordinary failure outcome=%+v error=%v", outcome, err)
	}
	for _, code := range []int{-1, 0, 2, 125} {
		outcome, err := classifyGoTestExit(code, "not predicate evidence")
		if err == nil || outcome.passed || !strings.Contains(err.Error(), "unsupported status") {
			t.Fatalf("status %d outcome=%+v error=%v", code, outcome, err)
		}
	}
}

func TestStrictGoTestEnvironmentHasNoAmbientSelectionSurface(t *testing.T) {
	directories := map[string]string{
		"HOME": "/private/home", "TMPDIR": "/private/tmp", "GOCACHE": "/private/cache",
		"GOMODCACHE": "/private/mod", "GOPATH": "/private/gopath",
		"XDG_CACHE_HOME": "/private/xdg",
	}
	environment := strictGoTestEnvironment("/exact/stage", "/exact/go", directories)
	joined := strings.Join(environment, "\n")
	for _, required := range []string{
		"PATH=/exact/go/bin", "PWD=/exact/stage", "GOENV=off", "GOWORK=off",
		"GOFLAGS=-mod=readonly", "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("strict environment omitted %q: %v", required, environment)
		}
	}
	if strings.Contains(joined, "NoSuchTest") {
		t.Fatalf("strict environment contains ambient test selection: %v", environment)
	}
}
