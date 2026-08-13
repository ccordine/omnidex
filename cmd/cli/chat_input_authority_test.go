package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestChatInstructionValidationPreservesNonBlankInput(t *testing.T) {
	t.Parallel()

	exact := "  keep leading and trailing authority\t  "
	if !isNonBlankChatInstruction(exact) {
		t.Fatal("non-blank chat instruction was rejected")
	}
	for _, blank := range []string{"", " ", "\t\n "} {
		if isNonBlankChatInstruction(blank) {
			t.Fatalf("blank chat instruction accepted: %q", blank)
		}
	}
}

func TestChatInputReaderPreservesLineBytesExceptScannerDelimiter(t *testing.T) {
	t.Parallel()

	reader := newChatInputReader(bufio.NewScanner(strings.NewReader("  exact line\t  \n")))
	line, eof, err := reader.readBlocking()
	if err != nil {
		t.Fatal(err)
	}
	if eof {
		t.Fatal("line was reported as EOF")
	}
	if line != "  exact line\t  " {
		t.Fatalf("line changed: %q", line)
	}
}

func TestInitialChatArgumentsPreserveArgumentContent(t *testing.T) {
	t.Parallel()

	exact, ok := initialChatInstruction([]string{"  first", "second  "})
	if !ok {
		t.Fatal("non-blank initial instruction was rejected")
	}
	if exact != "  first second  " {
		t.Fatalf("initial instruction=%q", exact)
	}
	if _, ok := initialChatInstruction([]string{" ", "\t"}); ok {
		t.Fatal("blank initial instruction was accepted")
	}
}
