package omni

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/gryph/omnidex/internal/hostbridge"
)

func (a *App) runHostService(args []string) error {
	if len(args) == 0 {
		a.printHostServiceUsage()
		return ExitCodeError{Code: 1}
	}

	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "help", "-h", "--help":
		a.printHostServiceUsage()
		return nil
	case "install":
		return a.runHostServiceInstall(args[1:])
	case "uninstall", "remove":
		return hostbridge.UninstallSystemdService()
	default:
		follow := false
		rest := args[1:]
		for len(rest) > 0 {
			switch rest[0] {
			case "-f", "--follow":
				follow = true
				rest = rest[1:]
			default:
				if strings.HasPrefix(rest[0], "-") {
					return fmt.Errorf("unknown host service option: %s", rest[0])
				}
				return fmt.Errorf("unexpected host service argument: %s", rest[0])
			}
		}
		return hostbridge.RunSystemdServiceAction(action, follow)
	}
}

func (a *App) runHostServiceInstall(args []string) error {
	fs := flag.NewFlagSet("host service install", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	listen := fs.String("listen", envOrDefault("HOST_AGENT_LISTEN", "0.0.0.0:8091"), "listen address")
	token := fs.String("token", strings.TrimSpace(os.Getenv("HOST_AGENT_TOKEN")), "optional bearer token")
	omniPath := fs.String("omni", "", "path to omni binary (default: current executable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected host service install argument(s): %s", strings.Join(fs.Args(), " "))
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd service install is only supported on Linux")
	}

	if err := hostbridge.InstallSystemdService(hostbridge.ServiceInstallOptions{
		OmniPath: *omniPath,
		Listen:   *listen,
		Token:    *token,
	}); err != nil {
		return err
	}

	paths, err := hostbridge.ResolveServicePaths()
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Installed %s user service.\n", hostbridge.HostBridgeServiceName)
	fmt.Fprintf(a.out, "  unit: %s\n", paths.UnitFile)
	fmt.Fprintf(a.out, "  env:  %s\n", paths.EnvFile)
	fmt.Fprintln(a.out, "  status: omni host service status")
	fmt.Fprintln(a.out, "  logs:   omni host service logs")
	return nil
}

func (a *App) printHostServiceUsage() {
	fmt.Fprintln(a.errOut, "usage:")
	fmt.Fprintln(a.errOut, "  omni host service install [--listen addr] [--token value] [--omni path]")
	fmt.Fprintln(a.errOut, "  omni host service uninstall")
	fmt.Fprintln(a.errOut, "  omni host service <start|stop|restart|status|enable|disable|logs>")
	fmt.Fprintln(a.errOut, "")
	fmt.Fprintln(a.errOut, "Installs a systemd user service so the host bridge starts automatically at login.")
	fmt.Fprintln(a.errOut, "Default listen is 0.0.0.0:8091 for Docker; edit ~/.config/omni/host-bridge.env to change.")
}
