package assemblyline

import (
	"strings"
	"testing"
)

func TestTypeScriptFunctionPolicyRailsGeneratedCode(t *testing.T) {
	contract := TypeScriptFunctionContract{
		Signature: "function Capability(): ReactElement", TSX: true,
		Policy: TypeScriptFunctionPolicy{
			RequiredCalls: []TypeScriptCallRequirement{{
				Callees:        []string{"writeChannel"},
				StringArgument: "capability_001", StringArgumentIndex: 1,
			}},
			RestrictedCalls: []TypeScriptCallRestriction{{
				Callees: []string{"writeChannel"}, StringArgumentIndex: 1,
				AllowedStringArguments: []string{"capability_001"},
			}},
			RequiredJSXElements:  []string{"section"},
			ForbiddenIdentifiers: []string{"ForbiddenHostAPI", "fetch"},
			TopLevelCalls:        []string{"useTopLevelResource"},
		},
	}
	valid := `function Capability(): ReactElement {
  writeChannel(runtime, 'capability_001');
  const value = 'ready';
  return <section>{value}</section>;
}`
	if _, err := ParseTypeScriptFunction(contract, valid); err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string]string{
		"foreign channel": strings.Replace(valid, "capability_001", "capability_999", 1),
		"wrong element":   strings.ReplaceAll(valid, "section", "div"),
		"browser global":  strings.Replace(valid, "return <section>", "const host = new ForbiddenHostAPI(); return <section>", 1),
		"nested hook": strings.Replace(
			valid, "return <section>", "useEffect(() => { useTopLevelResource(handleValue); }, []); return <section>", 1,
		),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseTypeScriptFunction(contract, raw); err == nil {
				t.Fatalf("accepted policy violation:\n%s", raw)
			}
		})
	}
}
