package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
)

func main() {
	if err := applyInvocationCWDFromEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "warn: %v\n", err)
	}

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	if cmd == "version" || cmd == "--version" || cmd == "-v" {
		runVersion(os.Args[2:])
		return
	}
	executable, err := os.Executable()
	if err != nil {
		die(fmt.Sprintf("resolve agent-cli executable: %v", err))
	}
	baseURL, err := resolveCoreURL(os.Getenv("CORE_URL"), executable, readManagedEnvironment)
	if err != nil {
		die(err.Error())
	}
	timeout := getenvDuration("CLI_TIMEOUT", 30*time.Second)
	apiClient := client.New(baseURL, timeout)

	if tryRunServiceShortcut(os.Args[1:], baseURL) {
		return
	}

	if strings.HasPrefix(cmd, "service:") {
		runServiceWithPreset(strings.TrimPrefix(cmd, "service:"), os.Args[2:], baseURL)
		return
	}

	switch cmd {
	case "chat":
		runChat(apiClient, os.Args[2:])
	case "run":
		runCoding(apiClient, os.Args[2:])
	case "list":
		runList(apiClient, os.Args[2:])
	case "show":
		runShow(apiClient, os.Args[2:])
	case "watch":
		runWatch(apiClient, os.Args[2:])
	case "interrupt":
		runInterrupt(apiClient, os.Args[2:])
	case "cancel":
		runCancel(apiClient, os.Args[2:])
	case "replan":
		runReplan(apiClient, os.Args[2:])
	case "remember":
		runRemember(apiClient, os.Args[2:])
	case "memory-candidates":
		runMemoryCandidates(apiClient, os.Args[2:])
	case "ingest":
		runIngest(apiClient, os.Args[2:])
	case "media-index":
		runMediaIndex(apiClient, os.Args[2:])
	case "media-search":
		runMediaSearch(os.Args[2:])
	case "browser-scan":
		runBrowserScan(os.Args[2:])
	case "screen-read":
		runScreenRead(os.Args[2:])
	case "audio-notes":
		runAudioNotes(apiClient, os.Args[2:])
	case "permissions":
		runPermissions(os.Args[2:])
	case "feedback":
		runFeedback(apiClient, os.Args[2:])
	case "build":
		runBuild(os.Args[2:])
	case "update":
		runUpdate(os.Args[2:])
	case "stash":
		runStash(os.Args[2:])
	case "uninstall":
		runUninstall(os.Args[2:])
	case "migrate:fresh":
		runMigrateFresh(apiClient, os.Args[2:], baseURL)
	case "status":
		runStatus(apiClient, os.Args[2:], baseURL)
	case "metrics":
		runMetrics(apiClient, os.Args[2:])
	case "core:status":
		runCoreStatus(os.Args[2:], baseURL)
	case "queue:status":
		runQueueStatus(apiClient, os.Args[2:], baseURL)
	case "ollama:status":
		runOllamaStatus(os.Args[2:])
	case "ollama:prewarm":
		runOllamaPrewarm(os.Args[2:])
	case "web:status":
		runWebStatus(os.Args[2:])
	case "service":
		runService(os.Args[2:], baseURL)
	case "config":
		runConfig(os.Args[2:])
	case "host":
		runHost(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}
