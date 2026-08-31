package main

import (
	"flag"
	"strings"

	"github.com/gryph/omnidex/internal/hostbridge"
)

func runHostServe(args []string) {
	fs := flag.NewFlagSet("host serve", flag.ExitOnError)
	listen := fs.String("listen", getenv("HOST_AGENT_LISTEN", "127.0.0.1:8091"), "listen address")
	token := fs.String("token", getenv("HOST_AGENT_TOKEN", ""), "optional bearer token")
	_ = fs.Parse(args)

	if err := hostbridge.RunServe(hostbridge.ServeOptions{
		Listen: strings.TrimSpace(*listen),
		Token:  strings.TrimSpace(*token),
	}); err != nil {
		die(err.Error())
	}
}
