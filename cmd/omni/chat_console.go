package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"golang.org/x/term"
)

var errChatInputRejected = errors.New("interactive input rejected")

type terminalReadWriter struct {
	reader io.Reader
	writer io.Writer
}

func (stream terminalReadWriter) Read(buffer []byte) (int, error) {
	return stream.reader.Read(buffer)
}

func (stream terminalReadWriter) Write(buffer []byte) (int, error) {
	return stream.writer.Write(buffer)
}

type chatConsole struct {
	terminal      *term.Terminal
	pasteReader   *bracketedPasteReader
	terminalFD    int
	terminalState *term.State
	closeOnce     sync.Once
	inputOverflow atomic.Bool
	promptMu      sync.Mutex
	prompt        string
}

func newChatConsole(
	ctx context.Context,
	input io.Reader,
	output,
	errorsOutput io.Writer,
) (*chatConsole, error) {
	if ctx == nil || input == nil || output == nil || errorsOutput == nil {
		return nil, fmt.Errorf("interactive console requires stdin, stdout, and stderr")
	}
	console := &chatConsole{terminalFD: -1}
	inputFile, outputFile, err := requireChatTerminalStreams(input, output)
	if err != nil {
		return nil, err
	}

	fd := int(inputFile.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("enter terminal raw mode: %w", err)
	}
	if err := enableTerminalInterruptSignal(fd); err != nil {
		_ = term.Restore(fd, state)
		return nil, err
	}
	pasteReader := newBracketedPasteReader(
		newTerminalContextReader(ctx, inputFile),
		model.MaxFreeFormTurnBytes,
	)
	interactive := term.NewTerminal(
		terminalReadWriter{
			reader: pasteReader,
			writer: output,
		},
		"you> ",
	)
	interactive.AutoCompleteCallback = console.boundTerminalInput
	interactive.SetBracketedPasteMode(true)
	if width, height, sizeErr := term.GetSize(int(outputFile.Fd())); sizeErr == nil {
		_ = interactive.SetSize(width, height)
	}
	console.terminal = interactive
	console.pasteReader = pasteReader
	console.terminalFD = fd
	console.terminalState = state
	console.prompt = "you> "
	return console, nil
}

func requireChatTerminalStreams(input io.Reader, output io.Writer) (*os.File, *os.File, error) {
	inputFile, inputOK := input.(*os.File)
	outputFile, outputOK := output.(*os.File)
	if !inputOK || !outputOK || !term.IsTerminal(int(inputFile.Fd())) ||
		!term.IsTerminal(int(outputFile.Fd())) {
		return nil, nil, fmt.Errorf("omni chat requires an interactive terminal on stdin and stdout")
	}
	return inputFile, outputFile, nil
}

func (console *chatConsole) Close() error {
	if console == nil {
		return nil
	}
	var closeErr error
	console.closeOnce.Do(func() {
		if console.terminal != nil {
			console.terminal.SetBracketedPasteMode(false)
		}
		if console.terminalState != nil && console.terminalFD >= 0 {
			closeErr = term.Restore(console.terminalFD, console.terminalState)
		}
	})
	if closeErr != nil {
		return fmt.Errorf("restore terminal state: %w", closeErr)
	}
	return nil
}

func (console *chatConsole) ReadLine() (string, bool, error) {
	if console == nil {
		return "", false, fmt.Errorf("interactive console is unavailable")
	}
	if console.terminal != nil {
		console.inputOverflow.Store(false)
		line, err := console.terminal.ReadLine()
		pasted, pasteOverflow, mixedPaste, invalidUTF8, unsafeText, exactPaste := console.pasteReader.consumeLineState()
		if console.inputOverflow.Load() || pasteOverflow || len(line) > model.MaxFreeFormTurnBytes {
			return "", false, fmt.Errorf(
				"%w: input exceeded the %d-byte terminal boundary and was not submitted",
				errChatInputRejected,
				model.MaxFreeFormTurnBytes,
			)
		}
		if pasted && mixedPaste {
			return "", false, fmt.Errorf(
				"%w: pasted input cannot be combined with typed terminal edits; the turn was not submitted",
				errChatInputRejected,
			)
		}
		if invalidUTF8 {
			return "", false, fmt.Errorf(
				"%w: terminal input was not valid UTF-8 and was not submitted",
				errChatInputRejected,
			)
		}
		if unsafeText {
			return "", false, fmt.Errorf(
				"%w: terminal input contained unsafe formatting controls and was not submitted",
				errChatInputRejected,
			)
		}
		if err != nil && !errors.Is(err, term.ErrPasteIndicator) {
			return "", false, err
		}
		if pasted {
			return exactPaste, true, nil
		}
		if errors.Is(err, term.ErrPasteIndicator) {
			return "", false, fmt.Errorf("terminal paste provenance was not preserved")
		}
		return line, false, err
	}
	return "", false, fmt.Errorf("interactive terminal is unavailable")
}

func (console *chatConsole) boundTerminalInput(
	line string,
	position int,
	key rune,
) (string, int, bool) {
	if !utf8.ValidRune(key) || key < ' ' {
		return "", 0, false
	}
	if console.inputOverflow.Load() {
		return line, position, true
	}
	keyBytes := utf8.RuneLen(key)
	if keyBytes > 0 && len(line)+keyBytes <= model.MaxFreeFormTurnBytes {
		return "", 0, false
	}
	console.inputOverflow.Store(true)
	return line, position, true
}

func (console *chatConsole) IsTerminal() bool {
	return console != nil && console.terminal != nil
}

func (console *chatConsole) SetPrompt(prompt string) error {
	if console == nil || prompt == "" {
		return fmt.Errorf("interactive prompt is required")
	}
	console.promptMu.Lock()
	defer console.promptMu.Unlock()
	if console.prompt == prompt {
		return nil
	}
	console.prompt = prompt
	if console.terminal == nil {
		return fmt.Errorf("interactive terminal is unavailable")
	}
	console.terminal.SetPrompt(prompt)
	_, err := console.terminal.Write(nil)
	return err
}

func (console *chatConsole) WriteOutput(value string) error {
	if console == nil {
		return fmt.Errorf("interactive console is unavailable")
	}
	value = safeConsoleText(value)
	if console.terminal == nil {
		return fmt.Errorf("interactive terminal is unavailable")
	}
	_, err := console.terminal.Write([]byte(value))
	return err
}

func (console *chatConsole) WriteError(value string) error {
	if console == nil {
		return fmt.Errorf("interactive console is unavailable")
	}
	value = safeConsoleText(value)
	if console.terminal == nil {
		return fmt.Errorf("interactive terminal is unavailable")
	}
	_, err := console.terminal.Write([]byte(value))
	return err
}

func safeConsoleText(value string) string {
	var rendered strings.Builder
	for _, character := range value {
		switch {
		case character == '\n' || character == '\t':
			rendered.WriteRune(character)
		case unsafeTerminalTextRune(character):
			if character <= '\uffff' {
				fmt.Fprintf(&rendered, "\\u%04x", character)
			} else {
				fmt.Fprintf(&rendered, "\\U%08x", character)
			}
		default:
			rendered.WriteRune(character)
		}
	}
	return rendered.String()
}

func unsafeTerminalTextRune(character rune) bool {
	return character < ' ' || character == '\x7f' ||
		character >= '\u0080' && character <= '\u009f' ||
		unicode.Is(unicode.Cf, character) || character == '\u2028' || character == '\u2029'
}
