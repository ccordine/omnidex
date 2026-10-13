package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// consoleTestScreen observes the physical cursor operations written through
// terminalReadWriter. It models only the VT commands emitted by x/term and
// known display widths of the Unicode probes used in the composer tests.
type consoleTestScreen struct {
	mu          sync.Mutex
	cells       [][]string
	row, column int
	wrapPending bool
	pending     []byte
	transcript  strings.Builder
}

func newConsoleTestScreen(width, height int) *consoleTestScreen {
	screen := &consoleTestScreen{cells: make([][]string, height)}
	for row := range screen.cells {
		screen.cells[row] = make([]string, width)
		for column := range screen.cells[row] {
			screen.cells[row][column] = " "
		}
	}
	return screen
}

func (screen *consoleTestScreen) Write(value []byte) (int, error) {
	screen.mu.Lock()
	defer screen.mu.Unlock()
	screen.transcript.Write(value)
	screen.pending = append(screen.pending, value...)
	for len(screen.pending) > 0 {
		if screen.pending[0] == '\x1b' {
			if len(screen.pending) < 2 {
				break
			}
			if screen.pending[1] != '[' {
				return 0, fmt.Errorf("screen observer received an unsupported escape")
			}
			end := 2
			for end < len(screen.pending) && (screen.pending[end] < 0x40 || screen.pending[end] > 0x7e) {
				end++
			}
			if end == len(screen.pending) {
				break
			}
			if err := screen.control(string(screen.pending[2:end]), screen.pending[end]); err != nil {
				return 0, err
			}
			screen.pending = screen.pending[end+1:]
			continue
		}
		if !utf8.FullRune(screen.pending) {
			break
		}
		character, count := utf8.DecodeRune(screen.pending)
		screen.pending = screen.pending[count:]
		switch character {
		case '\r':
			screen.column, screen.wrapPending = 0, false
		case '\n':
			screen.row++
			screen.wrapPending = false
			screen.scroll()
		default:
			screen.print(character)
		}
	}
	return len(value), nil
}

func (screen *consoleTestScreen) control(parameter string, command byte) error {
	amount := 1
	if parameter != "" {
		parsed, err := strconv.Atoi(parameter)
		if err != nil {
			return fmt.Errorf("screen observer received invalid CSI %q", parameter)
		}
		amount = parsed
	}
	screen.wrapPending = false
	switch command {
	case 'A':
		screen.row = max(0, screen.row-amount)
	case 'B':
		screen.row = min(len(screen.cells)-1, screen.row+amount)
	case 'C':
		screen.column = min(len(screen.cells[0])-1, screen.column+amount)
	case 'D':
		screen.column = max(0, screen.column-amount)
	case 'K':
		if parameter != "" && parameter != "0" {
			return fmt.Errorf("screen observer received unsupported erase-line mode %q", parameter)
		}
		screen.erase(screen.row, screen.column)
	case 'J':
		if parameter != "" && parameter != "0" {
			return fmt.Errorf("screen observer received unsupported erase-display mode %q", parameter)
		}
		screen.erase(screen.row, screen.column)
		for row := screen.row + 1; row < len(screen.cells); row++ {
			screen.erase(row, 0)
		}
	default:
		return fmt.Errorf("screen observer received unsupported CSI %c", command)
	}
	return nil
}

func (screen *consoleTestScreen) print(character rune) {
	if unicode.Is(unicode.Mn, character) {
		if screen.column > 0 {
			screen.cells[screen.row][screen.column-1] += string(character)
		}
		return
	}
	width := 1
	if character == '界' || character == '🙂' {
		width = 2
	}
	columns := len(screen.cells[0])
	if screen.wrapPending || screen.column+width > columns {
		screen.row++
		screen.column, screen.wrapPending = 0, false
		screen.scroll()
	}
	screen.cells[screen.row][screen.column] = string(character)
	if width == 2 {
		screen.cells[screen.row][screen.column+1] = ""
	}
	screen.column += width
	if screen.column == columns {
		screen.column--
		screen.wrapPending = true
	}
}

func (screen *consoleTestScreen) erase(row, column int) {
	for column < len(screen.cells[row]) {
		screen.cells[row][column] = " "
		column++
	}
}

func (screen *consoleTestScreen) scroll() {
	if screen.row < len(screen.cells) {
		return
	}
	copy(screen.cells, screen.cells[1:])
	screen.cells[len(screen.cells)-1] = make([]string, len(screen.cells[0]))
	screen.row = len(screen.cells) - 1
	screen.erase(screen.row, 0)
}

func (screen *consoleTestScreen) snapshot() ([]string, int, int, string) {
	screen.mu.Lock()
	defer screen.mu.Unlock()
	rows := make([]string, len(screen.cells))
	for row, cells := range screen.cells {
		rows[row] = strings.TrimRight(strings.Join(cells, ""), " ")
	}
	return rows, screen.row, screen.column, screen.transcript.String()
}
