package main

import "testing"

func TestParseFreshConfirmationInput(t *testing.T) {
	if !parseFreshConfirmationInput("fresh\n") {
		t.Fatalf("expected fresh confirmation token to pass")
	}
	if !parseFreshConfirmationInput("  FrEsH  ") {
		t.Fatalf("expected case-insensitive fresh token to pass")
	}
	if parseFreshConfirmationInput("yes") {
		t.Fatalf("did not expect generic yes to pass fresh confirmation")
	}
	if parseFreshConfirmationInput("") {
		t.Fatalf("did not expect empty confirmation to pass")
	}
}
