package codingagent

import (
	"context"
	"testing"
)

func TestTaskFileAccessEnforcesCompiledPacket(t *testing.T) {
	access := taskFileWriter{environment: fixtureEnvironment{}, reads: []string{"read.txt", "write.txt", "delete.txt"}, files: []string{"write.txt"}, deletes: []string{"delete.txt"}}
	if _, err := access.ReadFile(context.Background(), "read.txt"); err != nil {
		t.Fatal(err)
	}
	if err := access.WriteFile(context.Background(), FileWrite{Path: "write.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := access.DeleteFile(context.Background(), FileDelete{Path: "delete.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := access.ReadFile(context.Background(), "other.txt"); err == nil {
		t.Fatal("read outside task packet was accepted")
	}
	if err := access.WriteFile(context.Background(), FileWrite{Path: "other.txt"}); err == nil {
		t.Fatal("write outside task packet was accepted")
	}
	if err := access.DeleteFile(context.Background(), FileDelete{Path: "other.txt"}); err == nil {
		t.Fatal("delete outside task packet was accepted")
	}
}
