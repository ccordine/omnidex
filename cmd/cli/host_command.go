package main

import (
	"fmt"
	"os"
)

func runHost(args []string) {
	if len(args) == 0 {
		printHostUsage()
		os.Exit(1)
	}
	switch args[0] {
	case "serve":
		runHostServe(args[1:])
	case "help", "-h", "--help":
		printHostUsage()
	default:
		die(fmt.Sprintf("unknown host subcommand %q (try: omni host serve)", args[0]))
	}
}

func printHostUsage() {
	fmt.Println("usage:")
	fmt.Println("  omni host serve [--listen addr] [--token value]")
	fmt.Println("")
	fmt.Println("Runs a host bridge so Dockerized core and the web UI can browse and pick real host directories.")
	fmt.Println("Set HOST_AGENT_URL in core, e.g. http://host.docker.internal:8091 when using Docker.")
}
