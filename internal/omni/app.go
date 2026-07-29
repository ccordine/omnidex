package omni

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/version"
)

type agentCLIRunner func(args []string, input io.Reader) error

// App is the small host-side front door. All AI work is owned by agent-core;
// this type only dispatches core commands and the few host utilities that must
// run outside the container.
type App struct {
	in       io.Reader
	out      io.Writer
	errOut   io.Writer
	agentCLI agentCLIRunner
}

func NewApp(in io.Reader, out, errOut io.Writer) *App {
	app := &App{in: in, out: out, errOut: errOut}
	app.agentCLI = app.executeAgentCLI
	return app
}

func (a *App) Run(args []string) error {
	if a == nil || a.in == nil || a.out == nil || a.errOut == nil {
		return fmt.Errorf("omni app requires stdin, stdout, and stderr")
	}
	if len(args) == 0 {
		if isInteractiveReader(a.in) {
			return a.runAgentMode(nil)
		}
		return a.runOneShot(nil)
	}

	switch args[0] {
	case "version", "--version", "-v":
		return a.printVersion(args[1:])
	case "help", "--help", "-h":
		a.printUsage()
		return nil
	case "update":
		return a.runUpdate(args[1:])
	case "migrate":
		return a.runMigrate(args[1:])
	case "host":
		return a.runHost(args[1:])
	case "chat":
		return a.runAgentMode(args[1:])
	case "run":
		return a.runOneShot(args[1:])
	case "ledger", "bench", "run:trace", "fastpath", "index", "map", "fingerprint", "patch", "ollama", "agent":
		return fmt.Errorf("omni %s was removed with the obsolete local-agent runtime; use omni chat, omni run, or an explicit agent-cli command", args[0])
	default:
		if isAgentCLIPassthroughCommand(args[0]) {
			return a.runAgentMode(args)
		}
		return a.runAgentMode(args)
	}
}

func (a *App) runOneShot(args []string) error {
	blob, err := io.ReadAll(a.in)
	if err != nil {
		return fmt.Errorf("read one-shot instruction: %w", err)
	}
	instruction := strings.TrimSpace(string(blob))
	if instruction == "" {
		return fmt.Errorf("one-shot instruction is required on stdin")
	}
	cliArgs := append([]string{"run"}, args...)
	cliArgs = append(cliArgs, instruction)
	return a.runAgentCLI(cliArgs, strings.NewReader(""))
}

func (a *App) printVersion(args []string) error {
	if len(args) == 0 {
		_, err := fmt.Fprintln(a.out, version.PrintName("omni"))
		return err
	}
	if len(args) != 1 || args[0] != "--json" {
		return fmt.Errorf("usage: omni version [--json]")
	}
	encoded, err := json.MarshalIndent(version.JSON(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode version: %w", err)
	}
	_, err = fmt.Fprintln(a.out, string(encoded))
	return err
}

func (a *App) printUsage() {
	fmt.Fprintln(a.errOut, "Usage:")
	fmt.Fprintln(a.errOut, "  omni chat [flags] [initial message]  start core-backed agent chat")
	fmt.Fprintln(a.errOut, "  omni run [flags]                    send stdin as one core-backed task")
	fmt.Fprintln(a.errOut, "  omni update [flags]                 update the host installation")
	fmt.Fprintln(a.errOut, "  omni migrate <command>              manage database migrations")
	fmt.Fprintln(a.errOut, "  omni host <command>                 manage the host bridge")
}

func isInteractiveReader(input io.Reader) bool {
	file, ok := input.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
