package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/gryph/omnidex/internal/hostbridge"
)

func runHostService(args []string) {
	if len(args) == 0 {
		printHostServiceUsage()
		os.Exit(1)
	}

	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "help", "-h", "--help":
		printHostServiceUsage()
	case "install":
		runHostServiceInstall(args[1:])
	case "uninstall", "remove":
		if err := hostbridge.UninstallSystemdService(); err != nil {
			die(err.Error())
		}
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
					die(fmt.Sprintf("unknown host service option: %s", rest[0]))
				}
				die(fmt.Sprintf("unexpected host service argument: %s", rest[0]))
			}
		}
		if err := hostbridge.RunSystemdServiceAction(action, follow); err != nil {
			die(err.Error())
		}
	}
}

func runHostServiceInstall(args []string) {
	fs := flag.NewFlagSet("host service install", flag.ExitOnError)
	listen := fs.String("listen", getenv("HOST_AGENT_LISTEN", "0.0.0.0:8091"), "listen address")
	token := fs.String("token", getenv("HOST_AGENT_TOKEN", ""), "optional bearer token")
	omniPath := fs.String("omni", "", "path to omni binary (default: current executable)")
	_ = fs.Parse(args)

	if runtime.GOOS != "linux" {
		die("systemd service install is only supported on Linux")
	}
	if err := hostbridge.InstallSystemdService(hostbridge.ServiceInstallOptions{
		OmniPath: *omniPath,
		Listen:   *listen,
		Token:    *token,
	}); err != nil {
		die(err.Error())
	}

	paths, err := hostbridge.ResolveServicePaths()
	if err != nil {
		die(err.Error())
	}
	fmt.Printf("Installed %s user service.\n", hostbridge.HostBridgeServiceName)
	fmt.Printf("  unit: %s\n", paths.UnitFile)
	fmt.Printf("  env:  %s\n", paths.EnvFile)
	fmt.Println("  status: omni host service status")
	fmt.Println("  logs:   omni host service logs")
}

func printHostServiceUsage() {
	fmt.Println("usage:")
	fmt.Println("  omni host service install [--listen addr] [--token value] [--omni path]")
	fmt.Println("  omni host service uninstall")
	fmt.Println("  omni host service <start|stop|restart|status|enable|disable|logs>")
	fmt.Println("")
	fmt.Println("Installs a systemd user service so the host bridge starts automatically at login.")
	fmt.Println("Default listen is 0.0.0.0:8091 for Docker; edit ~/.config/omni/host-bridge.env to change.")
}
