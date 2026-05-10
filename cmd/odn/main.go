package main

import (
	"fmt"
	"os"

	"github.com/gryph/omnidex/internal/odn"
)

func main() {
	app := odn.NewApp(os.Stdin, os.Stdout, os.Stderr)
	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
