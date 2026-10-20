//go:build darwin || linux

package main

import (
	"os"
	"os/signal"
	"syscall"
)

func watchChatResize() (<-chan os.Signal, func()) {
	resizes := make(chan os.Signal, 1)
	signal.Notify(resizes, syscall.SIGWINCH)
	return resizes, func() { signal.Stop(resizes) }
}
