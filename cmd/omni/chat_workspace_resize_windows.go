//go:build windows

package main

import "os"

// Windows terminal dimensions are observed by the existing display tick.
func watchChatResize() (<-chan os.Signal, func()) { return nil, func() {} }
