//go:build windows

package main

import "fmt"

func requireDirectHostPathPlatform() error {
	return fmt.Errorf(
		"omni chat direct-write authority requires one Unix absolute host path mounted at the identical path in Omnidex core; Windows drive paths are unsupported",
	)
}
