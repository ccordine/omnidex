package changeapply

import (
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestMutableTargetRejectsSymlinkAndReservedContentWithoutFallback(t *testing.T) {
	t.Parallel()
	target := repositoryfacts.ChangeTarget{
		SymbolID:  "symbol_" + strings.Repeat("a", 64),
		StartByte: 0, EndByte: 1, ExpectedFileSHA256: strings.Repeat("b", 64),
	}
	file := repositoryfacts.File{
		Path: "linked.go", Kind: repositoryfacts.EntrySymlink,
		SHA256: target.ExpectedFileSHA256, Size: 1, Mode: 0o777,
	}
	if err := validateMutableTarget(file, target); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink target error=%v", err)
	}
	file.Kind = repositoryfacts.EntryRegular
	file.Path = ".omni/generated.go"
	if err := validateMutableTarget(file, target); err == nil || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("reserved target error=%v", err)
	}
}
