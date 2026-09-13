package worker

import (
	"fmt"
	"strings"
	"testing"
)

func TestGoDockerVerificationPublishesOnlyPassingTasks(t *testing.T) {
	for _, fixture := range []struct{ name, implementation, example, predicate string }{
		{"text output", `return "ready"`, `return []string{}`, `return "ready"`},
		{"argument selection", `return arguments[1]`, `return []string{"first", "second"}`, `return "second"`},
	} {
		for _, broken := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/broken=%t", fixture.name, broken), func(t *testing.T) {
				program := testExactGoSieveProgram(t)
				body := fixture.implementation
				if broken {
					body = `return "incorrect"`
				}
				program.Generated["feature.001"] = "func Feature001(arguments []string) string {\n" + body + "\n}"
				program.Generated["acceptance.input.001"] = "func MakeExampleInputFeature001() []string {\n" + fixture.example + "\n}"
				program.Generated["acceptance.001"] = "func ExpectedFeature001(arguments []string) string {\n" + fixture.predicate + "\n}"
				// Assembly applies the registered Go formatter; these are ordinary
				// declaration fixtures, with no live model or workload prompt.
				for _, record := range runConstructedDockerPublicationFixture(t, program, broken, false) {
					if *record.ContainerNetworkEnabled {
						t.Fatal("offline Go verification enabled networking")
					}
					if record.Status == "exit_failed" && !strings.Contains(string(record.Stdout), "behavior mismatch") {
						t.Fatalf("Go failed before its behavioral assertion: %s %s", record.Stdout, record.Stderr)
					}
				}
			})
		}
	}
}
