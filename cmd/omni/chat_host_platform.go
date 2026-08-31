//go:build !windows

package main

func requireDirectHostPathPlatform() error {
	return nil
}
