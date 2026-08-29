package assemblyline

import "testing"

func TestExactSourceDeclarationPortableProjectionBindsCompleteResponse(t *testing.T) {
	t.Parallel()
	raw := "function value() {\r\n  return 1;\r\n}"
	projection, err := NewExactSourceDeclarationPortableResultProjection(raw)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Kind != PortableResultProjectionSourceDeclaration ||
		projection.Source != raw || projection.StartByte != 0 ||
		projection.EndByte != len(raw) || projection.RawBytes != len(raw) ||
		projection.DiscardedBytes != 0 ||
		projection.SourceResponseSHA256 != projection.SourceSHA256 {
		t.Fatalf("projection=%+v", projection)
	}
	if err := projection.ValidateFor(raw); err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateFor(raw + "changed"); err == nil {
		t.Fatal("projection accepted a different raw response")
	}
}
