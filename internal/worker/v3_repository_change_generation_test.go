package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestExistingRepositoryGoModificationInputProjectsOnlyDirectCapabilities(t *testing.T) {
	t.Parallel()
	current := "func Value() int { return Helper() }"
	digest := sha256.Sum256([]byte(current))
	target := repositoryfacts.ChangeTarget{
		SymbolID: "symbol-opaque", Kind: "function", Signature: "func Value() int",
		ExpectedDeclarationSHA256: hex.EncodeToString(digest[:]), RequirementQuote: "return two",
		DirectCapabilities: []repositoryfacts.DirectCapability{{
			SymbolID: "symbol-helper", Name: "Helper", Signature: "func Helper() int",
			SourceSHA256: strings.Repeat("a", 64), PermittedSymbols: []string{"Helper"},
		}},
	}
	input, err := existingRepositoryGoModificationInput(target, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Capabilities) != 1 || input.Capabilities[0] != "func Helper() int" ||
		len(input.PermittedSymbols) != 1 || input.PermittedSymbols[0] != "Helper" {
		t.Fatalf("fragment input=%+v", input)
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"symbol-opaque", "symbol-helper", "/workspace", "value.go"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("fragment input leaked %q: %s", forbidden, raw)
		}
	}
}

func TestExistingRepositoryGoModificationInputRejectsStaleDeclaration(t *testing.T) {
	t.Parallel()
	target := repositoryfacts.ChangeTarget{
		SymbolID: "symbol-opaque", Signature: "func Value() int",
		ExpectedDeclarationSHA256: strings.Repeat("a", 64), RequirementQuote: "return two",
	}
	if _, err := existingRepositoryGoModificationInput(target, "func Value() int { return 1 }"); err == nil ||
		!strings.Contains(err.Error(), "declaration hash is stale") {
		t.Fatalf("stale declaration error=%v", err)
	}
}
