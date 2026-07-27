package omni

import (
	"reflect"
	"strings"
	"testing"
)

func TestMutationWriteTargetPathsParsesQuotedRedirectionTarget(t *testing.T) {
	targets, err := mutationWriteTargetPaths(`printf 'export default function App(){ return ">"; }' > "src/App file.js"`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"src/App file.js"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
}

func TestMutationWriteTargetPathsParsesAppendAndTouchTargets(t *testing.T) {
	targets, err := mutationWriteTargetPaths(`printf 'next' >> src/App.js`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"src/App.js"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
}

func TestMutationWriteTargetPathsRejectsMissingRedirectionTarget(t *testing.T) {
	_, err := mutationWriteTargetPaths(`printf 'next' >`)
	if err == nil || !strings.Contains(err.Error(), "missing target") {
		t.Fatalf("expected explicit missing-target error, got %v", err)
	}
}

func TestMutationWriteTargetPathsRejectsUnterminatedQuotedTarget(t *testing.T) {
	_, err := mutationWriteTargetPaths(`printf next > "src/App.js`)
	if err == nil || !strings.Contains(err.Error(), "unterminated quote") {
		t.Fatalf("expected explicit quote error, got %v", err)
	}
}

func TestMutationWriteTargetPathsIgnoresFileDescriptorDuplication(t *testing.T) {
	targets, err := mutationWriteTargetPaths(`npm test 2>&1`)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Fatalf("file descriptor duplication is not a file target: %#v", targets)
	}
}

func TestMutationWriteTargetPathsIgnoresHTMLRedirectionsInsideHeredoc(t *testing.T) {
	command := `cat > index.html <<'HTML'
<!doctype html>
<html><body><div id="root"></div><script type="module" src="/src/main.tsx"></script></body></html>
HTML`
	targets, err := mutationWriteTargetPaths(command)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"index.html"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
}

func TestMutationWriteTargetPathsIgnoresJSXRedirectionsInsideHeredoc(t *testing.T) {
	command := `cat > src/App.tsx <<'TS'
export default function App() {
  return <main><h1>Smoke</h1></main>;
}
TS`
	targets, err := mutationWriteTargetPaths(command)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"src/App.tsx"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
}

func TestMutationWriteTargetPathsRejectsUnterminatedHeredoc(t *testing.T) {
	_, err := mutationWriteTargetPaths("cat > index.html <<'HTML'\n<html>")
	if err == nil || !strings.Contains(err.Error(), "unterminated heredoc") {
		t.Fatalf("expected explicit unterminated-heredoc error, got %v", err)
	}
}
