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

	baseURL := getenv("CORE_URL", "http://localhost:8090")
	timeout := getenvDuration("CLI_TIMEOUT", 30*time.Second)
	apiClient := client.New(baseURL, timeout)

	if tryRunServiceShortcut(os.Args[1:]) {
		return
	}

	cmd := os.Args[1]
	if cmd == "version" || cmd == "--version" || cmd == "-v" {
		runVersion(os.Args[2:])
		return
	}
	if strings.HasPrefix(cmd, "service:") {
		runServiceWithPreset(strings.TrimPrefix(cmd, "service:"), os.Args[2:])
		return
	}

	switch cmd {
	case "enqueue":
		runEnqueue(apiClient, os.Args[2:])
	case "chat":
		runChat(apiClient, os.Args[2:])
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
	case "continue":
		runContinueJob(apiClient, os.Args[2:])
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
	case "research":
		runResearch(apiClient, os.Args[2:])
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
		runMigrateFresh(apiClient, os.Args[2:])
	case "status":
		runStatus(apiClient, os.Args[2:])
	case "metrics":
		runMetrics(apiClient, os.Args[2:])
	case "core:status":
		runCoreStatus(os.Args[2:])
	case "queue:status":
		runQueueStatus(apiClient, os.Args[2:])
	case "ollama:status":
		runOllamaStatus(os.Args[2:])
	case "web:status":
		runWebStatus(os.Args[2:])
	case "service":
		runService(os.Args[2:])
	case "config":
		runConfig(os.Args[2:])
	case "host":
		runHost(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}
