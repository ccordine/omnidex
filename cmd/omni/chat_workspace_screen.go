package main

import (
	"fmt"
	"strconv"
	"strings"
)

const workspaceScrollbackLines = 4096

// The existing x/term editor owns input. This is its display surface, not a
// second editor or a source of conversation/workflow state.
type workspaceScreen struct {
	lines              [][]rune
	soft               []bool
	row, column, width int
	wrap               bool
	dropped            int
}

func newWorkspaceScreen(width int) workspaceScreen {
	return workspaceScreen{lines: [][]rune{{}}, soft: []bool{false}, width: max(1, width)}
}

func (screen *workspaceScreen) ensureRow() {
	for len(screen.lines) <= screen.row {
		screen.lines = append(screen.lines, nil)
		screen.soft = append(screen.soft, false)
	}
	if len(screen.lines) > workspaceScrollbackLines {
		drop := len(screen.lines) - workspaceScrollbackLines
		screen.lines = append([][]rune(nil), screen.lines[drop:]...)
		screen.soft = append([]bool(nil), screen.soft[drop:]...)
		screen.row -= drop
		screen.dropped += drop
	}
}

func (screen *workspaceScreen) put(character rune) {
	switch character {
	case '\r':
		screen.column, screen.wrap = 0, false
	case '\n':
		screen.soft[screen.row] = false
		screen.row++
		screen.wrap = false
		screen.ensureRow()
	case '\b':
		screen.column = max(0, screen.column-1)
	case '\t':
		for count := 8 - screen.column%8; count > 0; count-- {
			screen.put(' ')
		}
	default:
		if character < ' ' {
			return
		}
		if screen.wrap {
			screen.soft[screen.row] = true
			screen.row++
			screen.column, screen.wrap = 0, false
		}
		screen.ensureRow()
		line := screen.lines[screen.row]
		for len(line) <= screen.column {
			line = append(line, ' ')
		}
		line[screen.column] = character
		screen.lines[screen.row] = line
		screen.column++
		if screen.column >= screen.width {
			screen.column, screen.wrap = screen.width-1, true
		}
	}
}

func (screen *workspaceScreen) control(parameters string, command byte) error {
	values := strings.Split(parameters, ";")
	numbers := make([]int, len(values))
	for index, value := range values {
		if value == "" {
			numbers[index] = 1
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid editor terminal control %q", parameters)
		}
		numbers[index] = parsed
	}
	amount := max(1, numbers[0])
	screen.wrap = false
	switch command {
	case 'A':
		screen.row = max(0, screen.row-amount)
	case 'B':
		screen.row += amount
	case 'C':
		screen.column = min(screen.width-1, screen.column+amount)
	case 'D':
		screen.column = max(0, screen.column-amount)
	case 'H', 'f':
		screen.row = amount - 1
		screen.column = 0
		if len(numbers) > 1 {
			screen.column = max(0, numbers[1]-1)
		}
	case 'K':
		screen.ensureRow()
		line := screen.lines[screen.row]
		screen.lines[screen.row] = line[:min(len(line), screen.column)]
		screen.soft[screen.row] = false
	case 'J':
		if parameters == "2" {
			*screen = newWorkspaceScreen(screen.width)
			break
		}
		screen.ensureRow()
		line := screen.lines[screen.row]
		screen.lines[screen.row] = line[:min(len(line), screen.column)]
		screen.lines = screen.lines[:screen.row+1]
		screen.soft = screen.soft[:screen.row+1]
		screen.soft[screen.row] = false
	case 'm': // Editor styling is supplied by the workspace theme.
	default:
		return fmt.Errorf("unsupported editor terminal control %q%c", parameters, command)
	}
	screen.ensureRow()
	return nil
}
