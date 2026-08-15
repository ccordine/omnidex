package assemblyline

import (
	"strings"
	"testing"
)

func TestTypeScriptCompilerRepairRegionSelectsExactASTOwner(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name, source, marker, want string
		tsx                        bool
	}{
		{
			name: "callback declaration",
			source: `function Toggle(index: number): void {
  const [muteList, setMuteList] = useState<boolean[]>([]);
  const handleMuteToggle = useCallback((selected: number) => {
    setMuteList(previous => {
      const nextMuted = !previous[selected];
      const next = [...previous];
      next[selected] = nextMuted;
      return next;
    });
    actions.set(` + "`mute_${selected}`" + `, nextMuted);
  }, [actions]);
  handleMuteToggle(index);
}`,
			marker: "nextMuted);",
			want: `  const handleMuteToggle = useCallback((selected: number) => {
    setMuteList(previous => {
      const nextMuted = !previous[selected];
      const next = [...previous];
      next[selected] = nextMuted;
      return next;
    });
    actions.set(` + "`mute_${selected}`" + `, nextMuted);
  }, [actions]);`,
		},
		{
			name: "TSX expression",
			tsx:  true,
			source: `function Panel(value: SharedValue): ReactElement {
  return (
    <output>
      {value}
    </output>
  );
}`,
			marker: "value}", want: "      {value}",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			line, column := typeScriptRepairTestLocation(t, fixture.source, fixture.marker)
			region, err := NewTypeScriptCompilerRepairRegion(
				fixture.source, fixture.tsx, line, column, typeScriptCompilerRepairTestBindings(),
			)
			if err != nil {
				t.Fatal(err)
			}
			if region.Kind != TypeScriptRepairRegionCompilerOwner || region.Source != fixture.want {
				t.Fatalf("region=%+v want source %q", region, fixture.want)
			}
			if !strings.Contains(region.Source, fixture.marker) {
				t.Fatalf("region omitted failing source %q: %q", fixture.marker, region.Source)
			}
		})
	}
}

func typeScriptCompilerRepairTestBindings() []TypeScriptRepairBinding {
	return []TypeScriptRepairBinding{{Name: "value", Type: "unknown"}}
}

func typeScriptRepairTestLocation(t *testing.T, source string, marker string) (int, int) {
	t.Helper()
	offset := strings.Index(source, marker)
	if offset < 0 {
		t.Fatalf("source lacks marker %q", marker)
	}
	prefix := source[:offset]
	line := strings.Count(prefix, "\n") + 1
	lastNewline := strings.LastIndex(prefix, "\n")
	return line, offset - lastNewline
}
