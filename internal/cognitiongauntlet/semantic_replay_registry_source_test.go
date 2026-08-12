package cognitiongauntlet

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticReplayRegistryEqualsQueueTraceKindAuthority(t *testing.T) {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate semantic replay registry test")
	}
	path := filepath.Join(filepath.Dir(current), "..", "queue", "cognition_sealed_trace_types.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	queueKinds := queueTraceKindCases(t, file)
	semanticKinds := cognitionreplay.SemanticSourceKinds()
	if len(queueKinds) != len(semanticKinds) {
		t.Fatalf("queue/semantic trace kind counts=%d/%d: %v / %v",
			len(queueKinds), len(semanticKinds), queueKinds, semanticKinds)
	}
	for index := range queueKinds {
		if queueKinds[index] != semanticKinds[index] {
			t.Fatalf("queue/semantic trace kind %d=%q/%q", index, queueKinds[index], semanticKinds[index])
		}
	}
}

func queueTraceKindCases(t *testing.T, file *ast.File) []string {
	t.Helper()
	var target *ast.FuncDecl
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "validCognitionTraceKind" {
			target = function
			break
		}
	}
	if target == nil {
		t.Fatal("queue validCognitionTraceKind authority is absent")
	}
	set := map[string]struct{}{}
	ast.Inspect(target.Body, func(node ast.Node) bool {
		clause, ok := node.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, expression := range clause.List {
			switch value := expression.(type) {
			case *ast.BasicLit:
				decoded, err := strconv.Unquote(value.Value)
				if err != nil {
					t.Fatal(err)
				}
				set[decoded] = struct{}{}
			case *ast.Ident:
				switch value.Name {
				case "CognitionTraceKindAcceptedFactMaterialization":
					set[queue.CognitionTraceKindAcceptedFactMaterialization] = struct{}{}
				case "CognitionTraceKindProviderBrainBootstrap":
					set[queue.CognitionTraceKindProviderBrainBootstrap] = struct{}{}
				case "CognitionTraceKindProposalMaterialization":
					set[queue.CognitionTraceKindProposalMaterialization] = struct{}{}
				default:
					t.Fatalf("unresolved queue trace kind identifier %q", value.Name)
				}
			}
		}
		return true
	})
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
