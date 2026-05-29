package omni

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/hostbridge"
)

func (a *App) runHost(args []string) error {
	if len(args) == 0 {
		a.printHostUsage()
		return ExitCodeError{Code: 1}
	}
	switch args[0] {
	case "serve":
		return a.runHostServe(args[1:])
	case "service":
		return a.runHostService(args[1:])
	case "help", "-h", "--help":
		a.printHostUsage()
		return nil
	default:
		return fmt.Errorf("unknown host subcommand %q (try: omni host serve or omni host service install; rebuild with `omni update` if missing)", args[0])
	}
}

func (a *App) printHostUsage() {
	fmt.Fprintln(a.errOut, "usage:")
	fmt.Fprintln(a.errOut, "  omni host serve [--listen addr] [--token value]")
	fmt.Fprintln(a.errOut, "  omni host service install|uninstall|start|stop|restart|status|logs")
	fmt.Fprintln(a.errOut, "")
	fmt.Fprintln(a.errOut, "Runs a host bridge so Dockerized core and the web UI can browse and pick real host directories.")
	fmt.Fprintln(a.errOut, "Set HOST_AGENT_URL in core, e.g. http://host.docker.internal:8091 when using Docker.")
}

func (a *App) runHostServe(args []string) error {
	fs := flag.NewFlagSet("host serve", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	listen := fs.String("listen", envOrDefault("HOST_AGENT_LISTEN", "127.0.0.1:8091"), "listen address")
	token := fs.String("token", strings.TrimSpace(os.Getenv("HOST_AGENT_TOKEN")), "optional bearer token")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected host serve argument(s): %s", strings.Join(fs.Args(), " "))
	}
	return hostbridge.RunServe(hostbridge.ServeOptions{
		Listen: *listen,
		Token:  *token,
	})
}
