package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"time"
)

func runBrowserScan(args []string) {
	fs := flag.NewFlagSet("browser-scan", flag.ExitOnError)
	withConsole := fs.Bool("console", false, "capture live JavaScript console events from debuggable tabs")
	emailWatch := fs.Bool("email-watch", false, "inspect email tabs and report newly visible inbox items since last scan")
	seconds := fs.Int("seconds", 2, "seconds to listen for console events per tab when --console is on")
	limit := fs.Int("limit", 50, "maximum console events to return when --console is on")
	defaultPorts := fs.String("ports", defaultBrowserProbePorts, "comma-separated debug ports to probe in addition to detected process flags")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	_ = fs.Parse(args)
	if err := ensureLocalPermission(permissionKeyBrowserInspect, "Allow inspecting local browser processes and tab metadata."); err != nil {
		die(err.Error())
	}
	warnings := make([]string, 0, 1)
	if *withConsole {
		if err := ensureLocalPermission(permissionKeyBrowserConsole, "Allow reading JavaScript console events from local browser DevTools endpoints."); err != nil {
			*withConsole = false
			warnings = append(warnings, err.Error())
		}
	}
	if *emailWatch {
		if err := ensureLocalPermission(permissionKeyBrowserConsole, "Allow reading inbox summaries from local browser email tabs via DevTools endpoints."); err != nil {
			die(err.Error())
		}
	}

	if *emailWatch {
		report, err := browserEmailReport(*defaultPorts)
		if err != nil {
			die(err.Error())
		}
		if len(warnings) > 0 {
			report["warnings"] = warnings
		}
		if *jsonOutput {
			payload, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				die(err.Error())
			}
			fmt.Println(string(payload))
			return
		}
		fmt.Println(browserEmailReportToText(report))
		return
	}

	report, err := browserScanReport(*withConsole, *seconds, *limit, *defaultPorts)
	if err != nil {
		die(err.Error())
	}
	if len(warnings) > 0 {
		report["warnings"] = warnings
	}

	if *jsonOutput {
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			die(err.Error())
		}
		fmt.Println(string(payload))
		return
	}

	fmt.Println(reportToText(report))
}

func browserScanReport(withConsole bool, seconds, limit int, defaultPorts string) (map[string]any, error) {
	if seconds <= 0 {
		seconds = 2
	}
	if limit <= 0 {
		limit = 50
	}

	processes := discoverBrowserProcesses()
	ports := mergePorts(extractDebugPorts(processes), parsePortList(defaultPorts))
	endpoints := discoverBrowserEndpoints(ports)

	report := map[string]any{
		"generated_at":   time.Now().Format(time.RFC3339),
		"process_count":  len(processes),
		"endpoint_count": len(endpoints),
		"processes":      processes,
		"endpoints":      endpoints,
	}

	if withConsole {
		events := collectConsoleEvents(endpoints, time.Duration(seconds)*time.Second, limit)
		report["console_event_count"] = len(events)
		report["console_events"] = events
	}

	return report, nil
}
