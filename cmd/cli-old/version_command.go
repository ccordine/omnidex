package main

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/version"
)

func runVersion(args []string) {
	jsonOut := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOut = true
		}
	}
	if jsonOut {
		payload, err := json.MarshalIndent(version.JSON(), "", "  ")
		if err != nil {
			die("encode version: " + err.Error())
		}
		fmt.Println(string(payload))
		return
	}
	fmt.Println(version.PrintName("omni"))
}
