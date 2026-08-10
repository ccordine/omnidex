package cognitiongauntlet

import "testing"

func TestRestartTraceRequiresIdenticalDurableState(t *testing.T) {
	digest := traceTestDigest("checkpoint")
	receipt, err := NewRestartTrace(3, digest, digest)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.StateIdentical || receipt.CompletedCycles != 3 {
		t.Fatalf("restart receipt=%#v", receipt)
	}
	if _, err := NewRestartTrace(3, digest, traceTestDigest("changed")); err == nil {
		t.Fatal("restart accepted changed durable state")
	}
	if _, err := NewRestartTrace(0, digest, digest); err == nil {
		t.Fatal("restart accepted a zero cycle boundary")
	}
}
