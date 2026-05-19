package odn

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func PromptPermissionMode(in io.Reader, out io.Writer, current PermissionMode) (PermissionMode, error) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Permission mode:")
	fmt.Fprintln(out, "  1) ask_permission (default; reads allowed, writes require approval)")
	fmt.Fprintln(out, "  2) full_access    (reads/writes allowed, fully audited)")
	if current == PermissionFull {
		fmt.Fprintln(out, "Current session mode: full_access")
	} else {
		fmt.Fprintln(out, "Current session mode: ask_permission")
	}
	fmt.Fprint(out, "Select mode [1/2, default 1]: ")

	reader := bufferedReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}

	selection := strings.TrimSpace(strings.ToLower(line))
	switch selection {
	case "", "1", "ask", "ask_permission":
		return PermissionAsk, nil
	case "2", "full", "full_access":
		return PermissionFull, nil
	default:
		return "", fmt.Errorf("invalid selection %q", selection)
	}
}

func PromptYesNo(in io.Reader, out io.Writer, prompt string) (bool, error) {
	fmt.Fprint(out, prompt)
	reader := bufferedReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}

	s := strings.TrimSpace(strings.ToLower(line))
	return s == "y" || s == "yes", nil
}

func PromptClarification(in io.Reader, out io.Writer, question string) (string, error) {
	fmt.Fprintf(out, "\nclarify> %s\nuser> ", strings.TrimSpace(question))
	reader := bufferedReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	answer := strings.TrimSpace(line)
	if err == io.EOF && answer == "" {
		return "", io.ErrUnexpectedEOF
	}
	return answer, nil
}

func bufferedReader(in io.Reader) *bufio.Reader {
	if reader, ok := in.(*bufio.Reader); ok {
		return reader
	}
	return bufio.NewReader(in)
}
