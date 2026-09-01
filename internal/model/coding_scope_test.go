package model

import "testing"

func TestCodingScopeModeValidate(t *testing.T) {
	for _, mode := range []CodingScopeMode{
		CodingScopeModeStrict,
		CodingScopeModeNormal,
		CodingScopeModeExpansive,
	} {
		if err := mode.Validate(); err != nil {
			t.Fatalf("validate %q: %v", mode, err)
		}
	}

	for _, mode := range []CodingScopeMode{"", "STRICT", "wide"} {
		if err := mode.Validate(); err == nil {
			t.Fatalf("expected %q to be rejected", mode)
		}
	}
}
