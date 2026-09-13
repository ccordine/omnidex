//go:build linux

package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"golang.org/x/sys/unix"
)

func TestGoEntryPointUsesArgumentsWithoutWaitingForTerminalEOF(t *testing.T) {
	for _, fixture := range []struct{ name, body, argument, want string }{
		{"numeric output", `return fmt.Sprint(6 * 7)`, "", "42\n"},
		{"text enclosure", `return "[" + input.Arguments[0] + "]"`, "sample", "[sample]\n"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			binary := buildGoEntryPointFixture(t, fixture.body)
			terminal := openGoEntryPointTestTerminal(t)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			arguments := []string{}
			if fixture.argument != "" {
				arguments = append(arguments, fixture.argument)
			}
			command := exec.CommandContext(ctx, binary, arguments...)
			command.Stdin = terminal
			output, err := command.CombinedOutput()
			if err != nil || string(output) != fixture.want {
				t.Fatalf("terminal execution: %v, context=%v, output=%q; want %q", err, ctx.Err(), output, fixture.want)
			}
		})
	}
}

func TestGoEntryPointRetainsRedirectedInput(t *testing.T) {
	binary := buildGoEntryPointFixture(t, `return input.StandardInput`)
	command := exec.CommandContext(t.Context(), binary, "supplied-argument")
	command.Stdin = strings.NewReader("redirected content")
	output, err := command.CombinedOutput()
	if err != nil || string(output) != "redirected content\n" {
		t.Fatalf("redirected input: %v, output=%q", err, output)
	}
}

func buildGoEntryPointFixture(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	request := []assemblyline.Requirement{{ID: "requirement_001"}}
	main, err := goCommandLineApplicationDocument(request, directCodingCapabilityGraph{}, directCodingResultValueKindPlan{"requirement_001": assemblyline.ApplicationResultText}, []string{"requirement_001"}, nil, directCodingInputSourcePlan{"requirement_001": assemblyline.ApplicationInputBoth})
	if err != nil {
		t.Fatal(err)
	}
	source := main.Preamble + "\n\n" + goCommandLineRuntimeDocument().Blocks[0].Static + "\n\n" +
		"func Feature001(input TaskInput) string { " + body + " }\n\n" + main.Blocks[0].Static
	for name, content := range map[string]string{"main.go": source, "go.mod": "module fixture\n\ngo 1.24.0\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(root, "application")
	command := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", binary, ".")
	command.Dir = root
	command.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOTOOLCHAIN=local")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build entry point: %v\n%s", err, output)
	}
	return binary
}

func openGoEntryPointTestTerminal(t *testing.T) *os.File {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { master.Close() })
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { slave.Close() })
	return slave
}
