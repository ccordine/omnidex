package worker

import (
	"strings"
	"testing"
)

func TestBrowserFeatureContractRestoresTaskNeutralInteractionSemantics(t *testing.T) {
	t.Parallel()

	for _, behavior := range []string{
		"Let a reader dismiss a notice.",
		"Let an operator increase a displayed value.",
	} {
		contract := genericBrowserFeatureContract(behavior, false)
		for _, required := range []string{
			behavior,
			"works on its first activation without unstated state, data, service, or setup prerequisites",
			"Unless the exact requirement says otherwise, its control remains present and enabled before and after activation",
			"Unless the exact requirement says otherwise, its observable result remains present with a stable accessible name",
			"Requirement-specific shared state initially has no keys",
			"Establish needed state through the supplied actions inside event handlers",
			"Every button explicitly performs a non-submitting action",
			"no direct capability dependency or capability prerequisite",
			"Use only the directly available declarations and identifiers",
		} {
			if !strings.Contains(contract, required) {
				t.Fatalf("browser contract for %q lacks %q:\n%s", behavior, required, contract)
			}
		}
	}
}

func TestBrowserFeatureContextOmitsMechanicallyAbsentCapabilityBinding(t *testing.T) {
	t.Parallel()

	signature := genericBrowserFeatureSignature("FeatureView", "FeatureViewProps", nil)
	projection := genericBrowserFeatureProjectionSource("FeatureViewProps", nil)
	api := genericBrowserFeatureProjectionAPI("FeatureViewProps", nil)
	for label, value := range map[string]string{
		"signature":  signature,
		"projection": projection,
		"api":        api,
	} {
		if strings.Contains(value, "capabilities") {
			t.Fatalf("%s exposed an absent capability binding:\n%s", label, value)
		}
	}
	if signature != "function FeatureView({ state, actions }: FeatureViewProps): ReactElement" {
		t.Fatalf("unexpected zero-capability signature %q", signature)
	}
}

func TestBrowserFeatureContextRetainsRequiredDirectCapabilityBinding(t *testing.T) {
	t.Parallel()

	dependencies := []directCodingCapabilityBinding{{CapabilityID: "capability_002"}}
	signature := genericBrowserFeatureSignature("FeatureView", "FeatureViewProps", dependencies)
	projection := genericBrowserFeatureProjectionSource("FeatureViewProps", dependencies)
	api := genericBrowserFeatureProjectionAPI("FeatureViewProps", dependencies)
	contract := genericBrowserFeatureContract("Show the required relation.", true)
	for label, value := range map[string]string{
		"signature":  signature,
		"projection": projection,
		"api":        api,
	} {
		if !strings.Contains(value, "capabilit") {
			t.Fatalf("%s omitted a required capability binding:\n%s", label, value)
		}
	}
	if !strings.Contains(contract, "listed direct capabilities are the complete capability set") {
		t.Fatalf("capability-bearing contract lacks completeness fact:\n%s", contract)
	}
}

func TestSourceContractsDoNotDefineResponseOrReceiptGrammar(t *testing.T) {
	t.Parallel()

	contracts := []string{
		genericBrowserFeatureContract("Provide the requested interaction.", false),
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
