package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
)

const freshMigrationTimeout = 2 * time.Minute
const freshConfirmationToken = "fresh"

func runMigrateFresh(c *client.Client, args []string) {
	fs := flag.NewFlagSet("migrate:fresh", flag.ExitOnError)
	assumeYes := fs.Bool("yes", false, "skip interactive confirmation prompt")
	_ = fs.Parse(args)

	if c == nil {
		die("core client is unavailable")
	}

	coreURL := getenv("CORE_URL", "http://localhost:8090")
	target := fmt.Sprintf("core(%s)", coreURL)
	if !*assumeYes {
		if !stdinIsInteractive() {
			die("migrate:fresh requires --yes in non-interactive mode")
		}
		fmt.Printf("warning: this will permanently delete all Omnidex data in %s\n", target)
		fmt.Printf("type %q to continue: ", freshConfirmationToken)
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			die(fmt.Sprintf("failed to read confirmation: %v", err))
		}
		if !parseFreshConfirmationInput(line) {
			die("migrate:fresh canceled")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), freshMigrationTimeout)
	defer cancel()

	if err := c.MigrateFresh(ctx); err != nil {
		die(fmt.Sprintf("migrate:fresh via core failed: %v", err))
	}
	fmt.Printf("migrate:fresh complete via core at %s\n", coreURL)
}

func parseFreshConfirmationInput(input string) bool {
	return strings.EqualFold(strings.TrimSpace(input), freshConfirmationToken)
}

func stdinIsInteractive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
