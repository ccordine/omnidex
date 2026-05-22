package main

import "testing"

func TestSolveCalculusDerivative(t *testing.T) {
	got, err := SolveCalculus("x^2", "derivative")
	if err != nil {
		t.Fatal(err)
	}
	if got.Result != "2x" {
		t.Fatalf("result = %q, want 2x", got.Result)
	}
}

func TestSolveCalculusIntegral(t *testing.T) {
	got, err := SolveCalculus("sin(x)", "integral")
	if err != nil {
		t.Fatal(err)
	}
	if got.Result != "-cos(x) + C" {
		t.Fatalf("result = %q, want -cos(x) + C", got.Result)
	}
}

func TestSolveCalculusRejectsUnsupportedExpression(t *testing.T) {
	if _, err := SolveCalculus("tan(x)", "derivative"); err == nil {
		t.Fatal("expected unsupported expression error")
	}
}
