package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func usage() {
	fmt.Println("usage: omni <command> [flags] [args]")
	fmt.Println("")
	fmt.Println("commands:")
	fmt.Println("  enqueue [--pipeline assistant|chat|story] [--agent omnidex|cursor|codex] [--agent-model m] [--cursor-model m] [--codex-model m] [--codex-reasoning-effort minimal|low|medium|high|xhigh] [--search-query text] [--session id] [--model-plan m] [--model-analyze m] [--model-response m] [--model-search m] [--model-tagger m] [--model-verify m] [--model-memory m] <instruction>")
	fmt.Println("  chat [--session id] [--agent omnidex|cursor|codex] [--agent-model m] [--cursor-model m] [--codex-model m] [--codex-reasoning-effort minimal|low|medium|high|xhigh] [--interval 2s] [--progress] [--verbose] [--max-chars 1200] [--model-plan m] [--model-analyze m] [--model-response m] [--model-search m] [--model-tagger m] [--model-verify m] [--model-memory m] [initial message]")
	fmt.Println("  run [--session id] [--model m] [--agent omnidex|cursor|codex] [--agent-model m] [--interval 2s] [--progress] [--verbose] [--max-chars 1200] <coding instruction>")
	fmt.Println("  list [--status status] [--limit N] [--offset N]")
	fmt.Println("  show [--history generations|steps|artifacts|evidence|claims|llm_calls] [--history-limit N] [--history-cursor token] <job-id>")
	fmt.Println("  watch [--interval 2s] [--progress] [--verbose] [--max-chars 1200] <job-id>")
	fmt.Println("  interrupt [--operation-id id] <job-id> <context text>")
	fmt.Println("  replan [--operation-id id] <job-id> <context text>")
	fmt.Println("  continue <job-id> <follow-up instruction>")
	fmt.Println("  cancel [--operation-id id] <job-id> <reason>")
	fmt.Println("  feedback [--operation-id id] <job-id> <text>")
	fmt.Println("  remember [--source name] [--kind episodic|procedural|instruction|preference|reference] [--tags a,b,c] <content>")
	fmt.Println("  memory-candidates <list|promote|reject> ...")
	fmt.Println("  ingest [--source name] [--kind reference] [--tags a,b,c] [--chunk-size N] [--overlap N] <file...>")
	fmt.Println("  media-index [--root dir] [--source media] [--kind reference] [--tags a,b,c] [--episode-limit N] [--lines-per-chunk N] [--include-no-subs] [--dry-run]")
	fmt.Println("  media-search [--root dir] [--context N] [--limit N] <query>")
	fmt.Println("  browser-scan [--console] [--email-watch] [--seconds N] [--limit N] [--ports csv] [--json]")
	fmt.Println("  host serve [--listen addr] [--token value]   host bridge for native directory picker + browse")
	fmt.Println("  host service install|uninstall|start|stop|restart|status|logs   systemd user service (Linux)")
	fmt.Println("  screen-read [--ocr] [--vision] [--prompt text] [--model name] [--base-url url] [--keep] [--json]")
	fmt.Println("  audio-notes [doctor|start|stop|status|list|search] ...")
	fmt.Println("  permissions [list|path|grant|deny|unset|reset|help] ...")
	fmt.Println("  build [build-core.sh flags]         run scripts/build-core.sh")
	fmt.Println("  update [update.sh flags]            run update.sh")
	fmt.Println("  stash [stash flags]                 git stash helper for Omnidex repo")
	fmt.Println("  uninstall [uninstall.sh flags]      run uninstall.sh")
	fmt.Println("  migrate:fresh [--yes]  reset the dedicated Omnidex schema and re-run sealed migrations via core")
	fmt.Println("  metrics <live|runs|models|playbooks|benchmarks|export>  query telemetry/benchmark metrics")
	fmt.Println("  status [--timeout 5s] [--queue-limit N] [--web-probe]  combined service status")
	fmt.Println("  core:status [--timeout 5s] [--core-url url]            core API health")
	fmt.Println("  queue:status [--timeout 5s] [--limit N] [--core-url url] queue sample counts")
	fmt.Println("  ollama:status [--timeout 5s] [--base-url url]          ollama connectivity + models")
	fmt.Println("  ollama:prewarm [--model m] [--num-ctx N] [--keep-alive 10m] [--json]  model load/offload profile")
	fmt.Println("  model:gauntlet <capability-relation|requirement-partition|requirement-partition-complete> --stable-model m --output file [flags]  offline advisory trial")
	fmt.Println("  web:status [--timeout 5s] [--probe] [--providers csv]  web search provider status")
	fmt.Println("  service [--service name] <up|down|restart|status|logs|docker-logs|start|stop|build|migrate:fresh> [options]")
	fmt.Println("  service:<name> <up|down|restart|status|logs|docker-logs|start|stop|build|migrate:fresh> [options]")
	fmt.Println("  --service <name> <up|down|restart|status|logs|docker-logs|start|stop|build|migrate:fresh> [options]")
	fmt.Println("  config [--file path] [--editor cmd] [--print]          open Omnidex .env config")
	fmt.Println("  version [--json]                         print Omnidex release version")
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func mergeTags(parts ...[]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 16)

	for _, list := range parts {
		for _, raw := range list {
			tag := strings.ToLower(strings.TrimSpace(raw))
			if tag == "" {
				continue
			}
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}

	return out
}
