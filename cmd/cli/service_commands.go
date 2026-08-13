package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
)

const defaultServiceName = "core"
const defaultServiceLogTail = 120

type serviceCommandOptions struct {
	Service     string
	Action      string
	Prefix      string
	ComposeFile string
	Follow      bool
	Build       bool
	AssumeYes   bool
	Tail        int
}

func tryRunServiceShortcut(args []string, coreURL string) bool {
	if len(args) == 0 {
		return false
	}
	first := strings.TrimSpace(args[0])
	if first == "--service" || first == "-s" || strings.HasPrefix(first, "--service=") {
		runServiceWithPreset("", args, coreURL)
		return true
	}
	return false
}

func runService(args []string, coreURL string) {
	runServiceWithPreset("", args, coreURL)
}

func runServiceWithPreset(presetService string, args []string, coreURL string) {
	opts, showHelp, err := parseServiceCommandArgs(args, presetService)
	if showHelp {
		printServiceCommandUsage()
		return
	}
	if err != nil {
		die(err.Error())
	}

	shouldRunFresh, err := serviceRunsCoreMigrateFresh(opts)
	if err != nil {
		die(err.Error())
	}
	if shouldRunFresh {
		timeout := getenvDuration("CLI_TIMEOUT", 30*time.Second)
		c := client.New(coreURL, timeout)
		freshArgs := []string{}
		if opts.AssumeYes {
			freshArgs = append(freshArgs, "--yes")
		}
		runMigrateFresh(c, freshArgs, coreURL)
		return
	}

	root, composeFile, err := resolveServiceComposeTarget(opts.Prefix, opts.ComposeFile)
	if err != nil {
		die(err.Error())
	}
	composeCmd, err := resolveComposeCommandPrefix()
	if err != nil {
		die(err.Error())
	}
	if strings.EqualFold(strings.TrimSpace(opts.Action), "docker-logs") {
		invocation, err := dockerLogsInvocationForService(opts, composeCmd, composeFile, root)
		if err != nil {
			die(err.Error())
		}
		runServiceInvocationOrExit(invocation, root)
		return
	}
	invocation, err := composeInvocationForService(opts, composeCmd, composeFile)
	if err != nil {
		die(err.Error())
	}

	runServiceInvocationOrExit(invocation, root)
}

func printServiceCommandUsage() {
	fmt.Println("usage:")
	fmt.Println("  omni service [--service name] <up|down|restart|status|logs|docker-logs|start|stop|build|migrate:fresh> [options]")
	fmt.Println("  omni service:<name> <up|down|restart|status|logs|docker-logs|start|stop|build|migrate:fresh> [options]")
	fmt.Println("  omni --service <name> <up|down|restart|status|logs|docker-logs|start|stop|build|migrate:fresh> [options]")
	fmt.Println("")
	fmt.Println("options:")
	fmt.Println("  --service, -s <name>      target service (default: core)")
	fmt.Println("  --core                    shorthand for --service core")
	fmt.Println("  --all                     shorthand for --service all")
	fmt.Println("  --prefix <path>           repo/install root containing docker-compose.yml")
	fmt.Println("  --compose-file <path>     compose file path (default: auto-detect)")
	fmt.Println("  --build                   include --build when action is up")
	fmt.Println("  --follow, -f              follow logs when action is logs")
	fmt.Println("  --tail <N>                logs tail line count (default: 120)")
	fmt.Println("  docker-logs               resolve container id and run `docker logs` for the service")
	fmt.Println("  --yes, -y                 skip confirmation prompt for migrate:fresh")
	fmt.Println("  -h, --help                show this help")
}

func parseServiceCommandArgs(args []string, presetService string) (serviceCommandOptions, bool, error) {
	opts := serviceCommandOptions{
		Service: strings.TrimSpace(presetService),
		Tail:    defaultServiceLogTail,
	}
	if opts.Service == "" {
		opts.Service = defaultServiceName
	}

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}

		if arg == "-h" || arg == "--help" {
			return opts, true, nil
		}
		if arg == "--build" {
			opts.Build = true
			continue
		}
		if arg == "-y" || arg == "--yes" {
			opts.AssumeYes = true
			continue
		}
		if arg == "-f" || arg == "--follow" {
			opts.Follow = true
			continue
		}
		if arg == "--core" {
			opts.Service = "core"
			continue
		}
		if arg == "--all" {
			opts.Service = "all"
			continue
		}

		if arg == "-s" || arg == "--service" {
			if i+1 >= len(args) {
				return opts, false, fmt.Errorf("%s requires a value", arg)
			}
			i++
			opts.Service = strings.TrimSpace(args[i])
			if opts.Service == "" {
				return opts, false, fmt.Errorf("%s requires a non-empty value", arg)
			}
			continue
		}
		if strings.HasPrefix(arg, "--service=") {
			opts.Service = strings.TrimSpace(strings.TrimPrefix(arg, "--service="))
			if opts.Service == "" {
				return opts, false, fmt.Errorf("--service requires a non-empty value")
			}
			continue
		}

		if arg == "--prefix" {
			if i+1 >= len(args) {
				return opts, false, errors.New("--prefix requires a value")
			}
			i++
			opts.Prefix = strings.TrimSpace(args[i])
			if opts.Prefix == "" {
				return opts, false, errors.New("--prefix requires a non-empty value")
			}
			continue
		}
		if strings.HasPrefix(arg, "--prefix=") {
			opts.Prefix = strings.TrimSpace(strings.TrimPrefix(arg, "--prefix="))
			if opts.Prefix == "" {
				return opts, false, errors.New("--prefix requires a non-empty value")
			}
			continue
		}

		if arg == "--compose-file" {
			if i+1 >= len(args) {
				return opts, false, errors.New("--compose-file requires a value")
			}
			i++
			opts.ComposeFile = strings.TrimSpace(args[i])
			if opts.ComposeFile == "" {
				return opts, false, errors.New("--compose-file requires a non-empty value")
			}
			continue
		}
		if strings.HasPrefix(arg, "--compose-file=") {
			opts.ComposeFile = strings.TrimSpace(strings.TrimPrefix(arg, "--compose-file="))
			if opts.ComposeFile == "" {
				return opts, false, errors.New("--compose-file requires a non-empty value")
			}
			continue
		}

		if arg == "--tail" {
			if i+1 >= len(args) {
				return opts, false, errors.New("--tail requires a value")
			}
			i++
			value, err := strconv.Atoi(strings.TrimSpace(args[i]))
			if err != nil || value < 0 {
				return opts, false, errors.New("--tail requires a non-negative integer")
			}
			opts.Tail = value
			continue
		}
		if strings.HasPrefix(arg, "--tail=") {
			value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, "--tail=")))
			if err != nil || value < 0 {
				return opts, false, errors.New("--tail requires a non-negative integer")
			}
			opts.Tail = value
			continue
		}

		if strings.HasPrefix(arg, "--") {
			return opts, false, fmt.Errorf("unknown option: %s", arg)
		}

		if opts.Action == "" {
			if strings.EqualFold(arg, "docker") && i+1 < len(args) && strings.EqualFold(strings.TrimSpace(args[i+1]), "logs") {
				opts.Action = "docker-logs"
				i++
				continue
			}
			action, ok := normalizeServiceAction(arg)
			if !ok {
				return opts, false, fmt.Errorf("invalid service action %q (use up|down|restart|status|logs|docker-logs|start|stop|build|migrate:fresh)", arg)
			}
			opts.Action = action
			continue
		}

		return opts, false, fmt.Errorf("unexpected argument: %s", arg)
	}

	if opts.Action == "" {
		return opts, false, errors.New("service action is required (use up|down|restart|status|logs|docker-logs|start|stop|build|migrate:fresh)")
	}
	if strings.TrimSpace(opts.Service) == "" {
		opts.Service = defaultServiceName
	}
	if opts.AssumeYes && opts.Action != "migrate:fresh" {
		return opts, false, errors.New("--yes is only valid with migrate:fresh")
	}
	return opts, false, nil
}
