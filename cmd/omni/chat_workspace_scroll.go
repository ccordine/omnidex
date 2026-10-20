package main

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Locate the source character at the top of a scrolled viewport before reflow.
// Soft wraps belong to the same source paragraph; hard newlines remain distinct.
func workspaceRowAnchor(screen *workspaceScreen, width, target int) (int, int) {
	base := 0
	for first := 0; first < len(screen.lines); {
		end := first
		var paragraph strings.Builder
		for {
			paragraph.WriteString(string(screen.lines[end]))
			end++
			if end == len(screen.lines) || !screen.soft[end-1] {
				break
			}
		}
		count := len(strings.Split(ansi.Hardwrap(paragraph.String(), max(1, width), true), "\n"))
		if target < base+count {
			prefix := []rune(ansi.Truncate(paragraph.String(), max(0, target-base)*max(1, width), ""))
			remaining := len(prefix)
			for row := first; row < end; row++ {
				if remaining <= len(screen.lines[row]) {
					return row, remaining
				}
				remaining -= len(screen.lines[row])
			}
			return end - 1, len(screen.lines[end-1])
		}
		base += count
		first = end
	}
	return screen.row, screen.column
}
