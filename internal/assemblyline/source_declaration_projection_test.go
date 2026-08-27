package assemblyline

import (
	"strings"
	"testing"
)

func TestSourceDeclarationPortableProjectionBindsExactResponseSpan(t *testing.T) {
	t.Parallel()
	raw := "transport prefix\r\nfunction value() {\r\n  return 1;\r\n}\r\ntransport suffix"
	source := "function value() {\r\n  return 1;\r\n}"
	startByte := strings.Index(raw, source)
	projection, err := NewSourceDeclarationPortableResultProjection(
		raw, source, startByte, startByte+len(source),
	)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Kind != PortableResultProjectionSourceDeclaration ||
		projection.Source != source || projection.Source != raw[startByte:projection.EndByte] ||
		projection.RawBytes != len(raw) ||
		projection.DiscardedBytes != len(raw)-len(source) {
		t.Fatalf("projection=%+v", projection)
	}
	if err := projection.ValidateFor(raw); err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateFor(raw + "changed"); err == nil {
		t.Fatal("projection accepted a different raw response")
	}
}
