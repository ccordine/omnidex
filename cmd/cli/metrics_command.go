package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
)

func runMetrics(c *client.Client, args []string) {
	if len(args) == 0 {
		printMetricsUsage()
		return
	}
	switch args[0] {
	case "live":
		runMetricsFetch(c, "/v1/metrics/live", args[1:])
	case "runs":
		runMetricsRuns(c, args[1:])
	case "models":
		runMetricsFetch(c, "/v1/metrics/models", args[1:])
	case "playbooks":
		runMetricsFetch(c, "/v1/metrics/playbooks", args[1:])
	case "benchmarks":
		runMetricsFetch(c, "/v1/metrics/benchmarks", args[1:])
	case "export":
		runMetricsExport(c, args[1:])
	default:
		printMetricsUsage()
	}
}

func printMetricsUsage() {
	fmt.Println("usage:")
	fmt.Println("  agent-cli metrics live")
	fmt.Println("  agent-cli metrics runs [--limit N]")
	fmt.Println("  agent-cli metrics models")
	fmt.Println("  agent-cli metrics playbooks")
	fmt.Println("  agent-cli metrics benchmarks")
	fmt.Println("  agent-cli metrics export --run <id>")
}

func runMetricsRuns(c *client.Client, args []string) {
	fs := flag.NewFlagSet("metrics runs", flag.ExitOnError)
	limit := fs.Int("limit", 50, "maximum runs to return")
	_ = fs.Parse(args)
	runMetricsFetch(c, fmt.Sprintf("/v1/metrics/runs?limit=%d", *limit), fs.Args())
}

func runMetricsExport(c *client.Client, args []string) {
	fs := flag.NewFlagSet("metrics export", flag.ExitOnError)
	runID := fs.String("run", "", "telemetry run id")
	_ = fs.Parse(args)
	if strings.TrimSpace(*runID) == "" {
		die("--run is required")
	}
	runMetricsFetch(c, "/v1/metrics/runs/"+strings.TrimSpace(*runID), fs.Args())
}

func runMetricsFetch(c *client.Client, path string, args []string) {
	if len(args) > 0 {
		die("unexpected metrics argument(s): " + strings.Join(args, " "))
	}
	ctx, cancel := context.WithTimeout(context.Background(), getenvDuration("CLI_TIMEOUT", 30*time.Second))
	defer cancel()
	raw, err := c.MetricsRaw(ctx, path)
	if err != nil {
		die(err.Error())
	}
	var pretty any
	if err := json.Unmarshal(raw, &pretty); err != nil {
		fmt.Println(string(raw))
		return
	}
	blob, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		die("encode metrics response: " + err.Error())
	}
	fmt.Println(string(blob))
}
