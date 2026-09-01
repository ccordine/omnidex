package worker

import (
	"strings"
	"testing"
)

func TestSourceContractsDoNotDefineResponseOrReceiptGrammar(t *testing.T) {
	t.Parallel()

	contracts := []string{
		genericBrowserFeatureContract("Provide the requested interaction."),
		genericBrowserAcceptanceContract("Verify the requested interaction."),
		goCommandLineFeatureContract("Provide the requested command behavior."),
		javaCommandLineFeatureContract("Provide the requested command behavior."),
		javaScriptCommandLineFeatureContract("Provide the requested command behavior."),
		rustCommandLineFeatureContract("Provide the requested command behavior."),
	}
	for _, contract := range contracts {
		lower := strings.ToLower(contract)
		for _, forbidden := range []string{
			"extra declaration",
			"no declarations",
			"receipt",
			"role_ordinal",
			"placeholder_hint",
			"the function body fully implements",
			"return one map<string, object>",
			"taskresult",
			"exitcode",
			"exit_code",
			"runtime.result",
			"one object with output",
			"top-level intrinsic jsx root",
			"fireevent",
			"getbyrole",
			"tohavetextcontent",
		} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("source contract exposed response or receipt grammar %q:\n%s", forbidden, contract)
			}
		}
	}
}
