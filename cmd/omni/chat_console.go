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
	"time"
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
	// Internal projection markers must never reach the physical terminal.
	rendered := displayPasteProjection.Replace(string(buffer))
	count, err := stream.writer.Write([]byte(rendered))
	if err == nil && count != len(rendered) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return 0, err
	}
	return len(buffer), nil
}

type chatConsole struct {
	workspace      *chatWorkspace
	terminal       *term.Terminal
	pasteReader    *bracketedPasteReader
	reviewInput    *planReviewInputRouter
	output         io.Writer
	terminalFD     int
	terminalState  *term.State
	closeOnce      sync.Once
	inputOverflow  atomic.Bool
	presentationMu sync.Mutex
	reviewActive   bool
	reviewView     string
	promptMu       sync.Mutex
	prompt         string
	activity       chatActivity
	activityDetail string
	activityFrame  int
	renderedPrompt string
	terminalWidth  int
	terminalHeight int
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
	console := &chatConsole{terminalFD: -1, terminalWidth: 80, terminalHeight: 24}
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
	width, height := 80, 24
	if w, h, sizeErr := term.GetSize(int(outputFile.Fd())); sizeErr == nil && w > 0 && h > 0 {
		width, height = w, h
	}
	workspace, err := newChatWorkspace(output, width, height)
	if err != nil {
		_ = term.Restore(fd, state)
		return nil, err
	}
	source := &workspaceInputSource{source: newTerminalContextReader(ctx, inputFile), view: workspace}
	reviewInput, err := newPlanReviewInputRouter(source)
	if err != nil {
		_ = workspace.Close()
		_ = term.Restore(fd, state)
		return nil, fmt.Errorf("construct plan-review terminal input: %w", err)
	}
	source.review = reviewInput.ReviewEnabled
	pasteReader := newBracketedPasteReader(reviewInput, model.MaxFreeFormTurnBytes)
	interactive := term.NewTerminal(
		terminalReadWriter{
			reader: pasteReader,
			writer: workspace,
		},
		"you> ",
	)
	interactive.AutoCompleteCallback = console.boundTerminalInput
	interactive.SetBracketedPasteMode(true)
	columns, rows, _ := workspace.Resize(width, height)
	_ = interactive.SetSize(columns, rows)
	console.terminalWidth, console.terminalHeight = columns, rows
	console.workspace = workspace
	console.terminal = interactive
	console.pasteReader = pasteReader
	console.reviewInput = reviewInput
	console.output = workspace
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
		console.presentationMu.Lock()
		if console.reviewInput != nil {
			console.reviewInput.DisableReview()
		}
		if console.reviewActive {
			if err := console.leavePlanReviewScreenLocked(); err != nil {
				closeErr = err
			}
		}
		console.presentationMu.Unlock()
		if console.terminal != nil {
			console.terminal.SetBracketedPasteMode(false)
		}
		if console.workspace != nil {
			closeErr = errors.Join(closeErr, console.workspace.Close())
		}
		if console.terminalState != nil && console.terminalFD >= 0 {
			if err := term.Restore(console.terminalFD, console.terminalState); err != nil && closeErr == nil {
				closeErr = err
			}
		}
	})
	if closeErr != nil {
		return fmt.Errorf("restore terminal state: %w", closeErr)
	}
	return nil
}

func (console *chatConsole) ReadLine() (string, bool, terminalInputAuthority, error) {
	if console == nil {
		return "", false, terminalInputAuthority{}, fmt.Errorf("interactive console is unavailable")
	}
	if console.terminal != nil {
		console.inputOverflow.Store(false)
		if console.workspace != nil {
			console.workspace.beginInput()
		}
		line, err := console.terminal.ReadLine()
		if console.workspace != nil {
			console.workspace.endInput()
		}
		pasted, pasteOverflow, invalidUTF8, unsafeText, authority := console.pasteReader.consumeLineState()
		composed := restorePasteProjection.Replace(line)
		pasted = pasted || composed != line
		line = composed
		if console.inputOverflow.Load() || pasteOverflow || len(line) > model.MaxFreeFormTurnBytes {
			return "", false, authority, fmt.Errorf(
				"%w: input exceeded the %d-byte terminal boundary and was not submitted",
				errChatInputRejected,
				model.MaxFreeFormTurnBytes,
			)
		}
		if invalidUTF8 {
			return "", false, authority, fmt.Errorf(
				"%w: terminal input was not valid UTF-8 and was not submitted",
				errChatInputRejected,
			)
		}
		if unsafeText {
			return "", false, authority, fmt.Errorf(
				"%w: terminal input contained unsafe formatting controls and was not submitted",
				errChatInputRejected,
			)
		}
		if err != nil && !errors.Is(err, term.ErrPasteIndicator) {
			return "", false, authority, err
		}
		if pasted {
			return line, true, authority, nil
		}
		if errors.Is(err, term.ErrPasteIndicator) {
			return "", false, authority, fmt.Errorf("terminal paste provenance was not preserved")
		}
		return line, false, authority, err
	}
	return "", false, terminalInputAuthority{}, fmt.Errorf("interactive terminal is unavailable")
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
	if keyBytes > 0 && len(restorePasteProjection.Replace(line))+keyBytes <= model.MaxFreeFormTurnBytes {
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
	console.presentationMu.Lock()
	defer console.presentationMu.Unlock()
	console.promptMu.Lock()
	console.prompt = prompt
	console.promptMu.Unlock()
	if console.terminal == nil {
		return fmt.Errorf("interactive terminal is unavailable")
	}
	return console.redrawPromptLocked(time.Now())
}

func (console *chatConsole) WriteOutput(value string) (resultErr error) {
	if console == nil {
		return fmt.Errorf("interactive console is unavailable")
	}
	value = safeConsoleText(value)
	if console.terminal == nil {
		return fmt.Errorf("interactive terminal is unavailable")
	}
	console.presentationMu.Lock()
	defer console.presentationMu.Unlock()
	if console.workspace != nil {
		console.workspace.beginTranscript(value)
		defer func() { resultErr = errors.Join(resultErr, console.workspace.endTranscript()) }()
	}
	if console.reviewActive {
		return console.writeAroundPlanReviewLocked(value)
	}
	_, err := console.terminal.Write([]byte(value))
	return err
}

func (console *chatConsole) WriteError(value string) error {
	return console.WriteOutput(value)
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
