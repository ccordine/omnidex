package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func tryRunServiceShortcut(args []string) bool {
	if len(args) == 0 {
		return false
	}
	first := strings.TrimSpace(args[0])
	if first == "--service" || first == "-s" || strings.HasPrefix(first, "--service=") {
		runServiceWithPreset("", args)
		return true
	}
	return false
}

func runService(args []string) {
	runServiceWithPreset("", args)
}

func runServiceWithPreset(presetService string, args []string) {
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
		baseURL := getenv("CORE_URL", "http://localhost:8090")
		timeout := getenvDuration("CLI_TIMEOUT", 30*time.Second)
		c := client.New(baseURL, timeout)
		freshArgs := []string{}
		if opts.AssumeYes {
			freshArgs = append(freshArgs, "--yes")
		}
		runMigrateFresh(c, freshArgs)
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

func normalizeServiceAction(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "up":
		return "up", true
	case "down":
		return "down", true
	case "restart":
		return "restart", true
	case "status", "ps":
		return "status", true
	case "logs", "log":
		return "logs", true
	case "docker-logs", "docker:logs", "dlogs":
		return "docker-logs", true
	case "start":
		return "start", true
	case "stop":
		return "stop", true
	case "build":
		return "build", true
	case "migrate:fresh", "migrate-fresh":
		return "migrate:fresh", true
	default:
		return "", false
	}
}

func serviceRunsCoreMigrateFresh(opts serviceCommandOptions) (bool, error) {
	if strings.TrimSpace(opts.Action) != "migrate:fresh" {
		return false, nil
	}

	serviceName := normalizeServiceName(opts.Service)
	if serviceTargetsAll(serviceName) {
		return false, errors.New("migrate:fresh requires --service core")
	}
	if serviceName != "core" {
		return false, fmt.Errorf("migrate:fresh is only supported for service core (got %q)", serviceName)
	}
	return true, nil
}

func resolveComposeCommandPrefix() ([]string, error) {
	if _, err := exec.LookPath("docker"); err == nil {
		if err := exec.Command("docker", "compose", "version").Run(); err == nil {
			return []string{"docker", "compose"}, nil
		}
	}
	if _, err := exec.LookPath("docker-compose"); err == nil {
		return []string{"docker-compose"}, nil
	}
	return nil, errors.New("docker compose is required but was not found (need `docker compose` or `docker-compose`)")
}

func resolveServiceComposeTarget(prefix, composeFile string) (string, string, error) {
	cleanPrefix := expandHomePath(strings.TrimSpace(prefix))
	if cleanPrefix != "" {
		if abs, err := filepath.Abs(cleanPrefix); err == nil {
			cleanPrefix = abs
		}
		cleanPrefix = filepath.Clean(cleanPrefix)
	}

	cleanComposeFile := expandHomePath(strings.TrimSpace(composeFile))
	if cleanComposeFile != "" {
		if !filepath.IsAbs(cleanComposeFile) {
			base := cleanPrefix
			if base == "" {
				base = currentWorkingDirectory()
			}
			cleanComposeFile = filepath.Join(base, cleanComposeFile)
		}
		abs, err := filepath.Abs(cleanComposeFile)
		if err != nil {
			return "", "", err
		}
		cleanComposeFile = filepath.Clean(abs)
		if !scriptFileExists(cleanComposeFile) {
			return "", "", fmt.Errorf("compose file not found: %s", cleanComposeFile)
		}
		return filepath.Dir(cleanComposeFile), cleanComposeFile, nil
	}

	searchRoots := []string{}
	if cleanPrefix != "" {
		searchRoots = append(searchRoots, cleanPrefix)
	} else {
		searchRoots = runtimeRootCandidates(
			strings.TrimSpace(os.Getenv(omniRuntimeDirEnv)),
			currentWorkingDirectory(),
			currentExecutablePath(),
		)
	}

	for _, root := range searchRoots {
		for _, name := range []string{"docker-compose.yml", "docker-compose.yaml"} {
			candidate := filepath.Join(root, name)
			if scriptFileExists(candidate) {
				return root, candidate, nil
			}
		}
	}

	if cleanPrefix != "" {
		return "", "", fmt.Errorf("no docker-compose.yml found under %s", cleanPrefix)
	}
	return "", "", errors.New("unable to locate docker-compose.yml; pass --prefix or --compose-file")
}

func composeInvocationForService(opts serviceCommandOptions, composeCmd []string, composeFile string) ([]string, error) {
	if len(composeCmd) == 0 {
		return nil, errors.New("compose command prefix is required")
	}
	composeFile = strings.TrimSpace(composeFile)
	if composeFile == "" {
		return nil, errors.New("compose file is required")
	}

	action, ok := normalizeServiceAction(opts.Action)
	if !ok {
		return nil, fmt.Errorf("unsupported service action: %s", opts.Action)
	}
	serviceName := normalizeServiceName(opts.Service)
	targetAll := serviceTargetsAll(serviceName)

	invocation := append([]string{}, composeCmd...)
	invocation = append(invocation, "-f", composeFile)
	switch action {
	case "up":
		invocation = append(invocation, "up", "-d", "--remove-orphans")
		if opts.Build {
			invocation = append(invocation, "--build")
		}
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	case "down":
		if targetAll {
			invocation = append(invocation, "down", "--remove-orphans")
		} else {
			invocation = append(invocation, "stop", serviceName)
		}
	case "restart":
		invocation = append(invocation, "restart")
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	case "status":
		invocation = append(invocation, "ps")
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	case "logs":
		invocation = append(invocation, "logs", "--tail", strconv.Itoa(maxInt(opts.Tail, 0)))
		if opts.Follow {
			invocation = append(invocation, "-f")
		}
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	case "start":
		invocation = append(invocation, "start")
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	case "stop":
		invocation = append(invocation, "stop")
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	case "build":
		invocation = append(invocation, "build")
		if !targetAll {
			invocation = append(invocation, serviceName)
		}
	default:
		return nil, fmt.Errorf("unsupported service action: %s", action)
	}

	return invocation, nil
}

func normalizeServiceName(value string) string {
	clean := strings.ToLower(strings.TrimSpace(value))
	if clean == "" {
		return defaultServiceName
	}
	return clean
}

func serviceTargetsAll(service string) bool {
	switch strings.ToLower(strings.TrimSpace(service)) {
	case "", "*", "all", "stack":
		return true
	default:
		return false
	}
}

func runServiceInvocationOrExit(invocation []string, workdir string) {
	if len(invocation) == 0 {
		die("service invocation is empty")
	}

	cmd := exec.Command(invocation[0], invocation[1:]...)
	cmd.Dir = strings.TrimSpace(workdir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		die(fmt.Sprintf("service command failed: %v", err))
	}
}

func dockerLogsInvocationForService(opts serviceCommandOptions, composeCmd []string, composeFile string, workdir string) ([]string, error) {
	serviceName := normalizeServiceName(opts.Service)
	if serviceTargetsAll(serviceName) {
		return nil, errors.New("docker-logs requires a specific service (example: --service core)")
	}

	containerID, err := resolveComposeServiceContainerID(composeCmd, composeFile, serviceName, workdir)
	if err != nil {
		return nil, err
	}

	return buildDockerLogsInvocation(containerID, opts.Tail, opts.Follow)
}

func resolveComposeServiceContainerID(composeCmd []string, composeFile string, serviceName string, workdir string) (string, error) {
	if len(composeCmd) == 0 {
		return "", errors.New("compose command prefix is required")
	}
	composeFile = strings.TrimSpace(composeFile)
	if composeFile == "" {
		return "", errors.New("compose file is required")
	}
	serviceName = normalizeServiceName(serviceName)
	if serviceName == "" || serviceTargetsAll(serviceName) {
		return "", errors.New("service name is required for docker-logs")
	}

	invocation := append([]string{}, composeCmd...)
	invocation = append(invocation, "-f", composeFile, "ps", "-q", serviceName)
	cmd := exec.Command(invocation[0], invocation[1:]...)
	cmd.Dir = strings.TrimSpace(workdir)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		reason := strings.TrimSpace(stderr.String())
		if reason == "" {
			reason = err.Error()
		}
		return "", fmt.Errorf("failed resolving container for service %q: %s", serviceName, reason)
	}

	containerID := firstNonEmptyLine(stdout.String())
	if containerID == "" {
		return "", fmt.Errorf("no running container found for service %q", serviceName)
	}
	return containerID, nil
}

func buildDockerLogsInvocation(containerID string, tail int, follow bool) ([]string, error) {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return nil, errors.New("container id is required for docker logs")
	}
	dockerBin, err := exec.LookPath("docker")
	if err != nil {
		return nil, errors.New("docker is required for docker-logs action")
	}

	invocation := []string{dockerBin, "logs", "--tail", strconv.Itoa(maxInt(tail, 0))}
	if follow {
		invocation = append(invocation, "-f")
	}
	invocation = append(invocation, containerID)
	return invocation, nil
}

func firstNonEmptyLine(raw string) string {
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		clean := strings.TrimSpace(line)
		if clean != "" {
			return clean
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func expandHomePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if raw == "~" {
		return os.Getenv("HOME")
	}
	if strings.HasPrefix(raw, "~/") {
		return filepath.Join(os.Getenv("HOME"), strings.TrimPrefix(raw, "~/"))
	}
	return raw
}
