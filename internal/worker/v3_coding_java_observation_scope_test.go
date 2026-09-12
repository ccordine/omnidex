package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestJavaObservationScopeUsesParsedTypesAndArguments(t *testing.T) {
	input := assemblyline.FragmentGenerationInput{
		Language: "java", Dialect: "Java 21", Signature: "static void verify()", Behavior: "Observe the supplied result.",
		Capabilities: []string{"static native Map<String, Object> run(Map<String, Object> input, Map<String, Object> dependencies);"},
	}
	for name, body := range map[string]string{
		"object equality": `Map<String, Object> result = run(Map.of(), Map.of()); assert result.get("output").equals("ready");`,
		"inferred local":  `var result = run(Map.of(), Map.of()); assert result.get("output").equals("ready");`,
		"commented call":  `Map<String, Object> result = run(/* input */ Map.of(), /* dependencies */ Map.of()); assert "ready".equals(result.get("output"));`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateDirectCodingJavaFragment(input, body); err != nil {
				t.Fatalf("rejected ordinary Java observation: %v", err)
			}
		})
	}
	for _, body := range []string{
		`var result = unavailable(); assert result.get("output").equals("ready");`,
		`var result = run(Map.of(), Map.of()); result.getClass();`,
		`var result = run(Map.of(), Map.of()); result.exec("command");`,
	} {
		if _, err := validateDirectCodingJavaFragment(input, body); err == nil {
			t.Fatalf("inference granted unavailable authority: %s", body)
		}
	}
}
