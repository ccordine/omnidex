package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"golang.org/x/term"
)

type blockingPlanReviewSource struct {
	gate      sync.Mutex
	closeOnce sync.Once
	values    chan []byte
	pauseNext atomic.Bool
	entered   chan struct{}
	release   chan struct{}
}

func newBlockingPlanReviewSource() *blockingPlanReviewSource {
	return &blockingPlanReviewSource{
		values: make(chan []byte, 4), entered: make(chan struct{}), release: make(chan struct{}),
	}
}

func (source *blockingPlanReviewSource) LockInput()   { source.gate.Lock() }
func (source *blockingPlanReviewSource) UnlockInput() { source.gate.Unlock() }

func (source *blockingPlanReviewSource) Read(destination []byte) (int, error) {
	source.LockInput()
	defer source.UnlockInput()
	return source.ReadInputLocked(destination)
}

func (source *blockingPlanReviewSource) ReadInputLocked(destination []byte) (int, error) {
	select {
	case value, ok := <-source.values:
		if !ok {
			return 0, io.EOF
		}
		if source.pauseNext.Swap(false) {
			close(source.entered)
			<-source.release
		}
		return copy(destination, value), nil
	case <-time.After(time.Millisecond):
		return 0, errPlanReviewInputSourceIdle
	}
}

func (source *blockingPlanReviewSource) Close() {
	source.closeOnce.Do(func() { close(source.values) })
}

func TestPlanReviewNoteBytesCannotBecomeAChatTurnBeforeHandoff(t *testing.T) {
	source, input := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = input.Close()
		_ = source.Close()
	})
	router := mustPlanReviewInputRouter(t, source)
	pasteReader := newBracketedPasteReader(router, model.MaxFreeFormTurnBytes)
	var output bytes.Buffer
	terminal := term.NewTerminal(
		terminalReadWriter{reader: pasteReader, writer: &output},
		"you> ",
	)
	console := &chatConsole{
		terminal: terminal, pasteReader: pasteReader, reviewInput: router,
		output: &output, terminalFD: -1, prompt: "you> ",
	}
	chatInputs, chatDone := readChatInput(ctx, console)
	router.EnableReview()
	reviewInputs := readPlanReviewInput(ctx, router)

	writePlanReviewInput(t, input, "Nmake this local-only\r/exit\r")
	select {
	case event := <-reviewInputs:
		if event.Err != nil || event.EOF || event.Key != planReviewKeyNote {
			t.Fatalf("review event = %#v, want Note", event)
		}
	case <-time.After(time.Second):
		t.Fatal("review Note key was not delivered")
	}
	select {
	case event := <-chatInputs:
		t.Fatalf("note bytes became an ordinary chat event before handoff: %#v", event)
	case <-time.After(2 * planReviewEscapeTimeout):
	}

	authority, err := router.BeginNoteInput()
	if err != nil {
		t.Fatalf("BeginNoteInput() error = %v", err)
	}
	select {
	case event := <-chatInputs:
		if event.Err != nil || event.EOF || event.Pasted ||
			event.Text != "make this local-only" || event.Authority != authority {
			t.Fatalf("note input event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("accepted note bytes did not reach the line editor")
	}
	if err := router.EndNoteInput(authority); err != nil {
		t.Fatalf("EndNoteInput() error = %v", err)
	}
	select {
	case event := <-chatInputs:
		if event.Err != nil || event.EOF || event.Pasted ||
			event.Text != "/exit" || event.Authority != authority {
			t.Fatalf("trailing plan-note input event = %#v", event)
		}
		console := &chatConsole{reviewInput: router}
		session := &chatSession{renderer: chatRenderer{console: console}}
		quit, acceptErr := session.acceptAuthorizedInput(event)
		if quit || !errors.Is(acceptErr, errChatInputRejected) {
			t.Fatalf("trailing plan-note event = quit %t, error %v", quit, acceptErr)
		}
	case <-time.After(time.Second):
		t.Fatal("trailing plan-note bytes were not delivered with their original authority")
	}

	cancel()
	_ = input.Close()
	select {
	case <-chatDone:
	case <-time.After(time.Second):
		t.Fatal("chat input reader did not stop")
	}
}

func TestBracketedMultilinePasteRemainsOneAuthorizedChatInput(t *testing.T) {
	source, input := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = input.Close()
		_ = source.Close()
	})
	router := mustPlanReviewInputRouter(t, source)
	pasteReader := newBracketedPasteReader(router, model.MaxFreeFormTurnBytes)
	var output bytes.Buffer
	terminal := term.NewTerminal(
		terminalReadWriter{reader: pasteReader, writer: &output},
		"you> ",
	)
	terminal.SetBracketedPasteMode(true)
	console := &chatConsole{
		terminal: terminal, pasteReader: pasteReader, reviewInput: router,
		output: &output, terminalFD: -1, prompt: "you> ",
	}
	chatInputs, chatDone := readChatInput(ctx, console)

	writePlanReviewInput(t, input, "\x1b[200~first\nsecond\x1b[201~\r/exit\r")
	select {
	case event := <-chatInputs:
		if event.Err != nil || event.EOF || !event.Pasted ||
			event.Text != "first\nsecond" ||
			event.Authority != router.CurrentInputAuthority() {
			t.Fatalf("multiline paste event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("multiline paste was not delivered as one input")
	}
	select {
	case event := <-chatInputs:
		if event.Err != nil || event.EOF || event.Pasted || event.Text != "/exit" ||
			event.Authority != router.CurrentInputAuthority() {
			t.Fatalf("post-paste typed event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("typed input after multiline paste was not independently delivered")
	}

	cancel()
	_ = input.Close()
	select {
	case <-chatDone:
	case <-time.After(time.Second):
		t.Fatal("chat input reader did not stop")
	}
}

func TestPlanNoteModeTransitionWaitsForInFlightBytesToRetainAuthority(t *testing.T) {
	source := newBlockingPlanReviewSource()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		source.Close()
	})
	router, err := newPlanReviewInputRouter(source)
	if err != nil {
		t.Fatal(err)
	}
	pasteReader := newBracketedPasteReader(router, model.MaxFreeFormTurnBytes)
	var output bytes.Buffer
	terminal := term.NewTerminal(
		terminalReadWriter{reader: pasteReader, writer: &output},
		"you> ",
	)
	console := &chatConsole{
		terminal: terminal, pasteReader: pasteReader, reviewInput: router,
		output: &output, terminalFD: -1, prompt: "you> ",
	}
	chatInputs, chatDone := readChatInput(ctx, console)
	router.EnableReview()
	reviewInputs := readPlanReviewInput(ctx, router)
	source.values <- []byte("N")
	select {
	case event := <-reviewInputs:
		if event.Err != nil || event.EOF || event.Key != planReviewKeyNote {
			t.Fatalf("review event = %#v, want Note", event)
		}
	case <-time.After(time.Second):
		t.Fatal("review Note key was not delivered")
	}
	authority, err := router.BeginNoteInput()
	if err != nil {
		t.Fatal(err)
	}
	source.values <- []byte("first note\r")
	select {
	case event := <-chatInputs:
		if event.Text != "first note" || event.Authority != authority {
			t.Fatalf("first note event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("first note was not delivered")
	}

	source.pauseNext.Store(true)
	source.values <- []byte("/exit\r")
	select {
	case <-source.entered:
	case <-time.After(time.Second):
		t.Fatal("source did not enter the controlled in-flight read")
	}
	endDone := make(chan error, 1)
	go func() { endDone <- router.EndNoteInput(authority) }()
	select {
	case err := <-endDone:
		t.Fatalf("note authority ended before in-flight bytes were routed: %v", err)
	case <-time.After(2 * planReviewEscapeTimeout):
	}
	close(source.release)
	select {
	case err := <-endDone:
		if err != nil {
			t.Fatalf("EndNoteInput() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("note authority did not end after the in-flight read was routed")
	}
	select {
	case event := <-chatInputs:
		if event.Text != "/exit" || event.Authority != authority {
			t.Fatalf("in-flight trailing note event = %#v", event)
		}
		session := &chatSession{renderer: chatRenderer{console: console}}
		quit, acceptErr := session.acceptAuthorizedInput(event)
		if quit || !errors.Is(acceptErr, errChatInputRejected) {
			t.Fatalf("in-flight trailing note event = quit %t, error %v", quit, acceptErr)
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight trailing note was not delivered with note authority")
	}

	source.Close()
	select {
	case <-chatDone:
	case <-time.After(time.Second):
		t.Fatal("chat input reader did not stop")
	}
}

func TestChatInputQueuedBeforeReviewCannotExecuteAfterReviewCloses(t *testing.T) {
	router := mustPlanReviewInputRouter(t, strings.NewReader(""))
	stale := router.CurrentInputAuthority()
	router.EnableReview()
	router.DisableReview()
	console := &chatConsole{reviewInput: router}
	session := &chatSession{renderer: chatRenderer{console: console}}
	quit, err := session.acceptAuthorizedInput(chatInput{
		Text: "/exit", Authority: stale,
	})
	if quit || !errors.Is(err, errChatInputRejected) {
		t.Fatalf("stale chat event = quit %t, error %v", quit, err)
	}
}

func TestPlanReviewInputRouterSwitchesModeWhileReadIsBlocked(t *testing.T) {
	source, input := io.Pipe()
	t.Cleanup(func() {
		_ = input.Close()
		_ = source.Close()
	})
	router := mustPlanReviewInputRouter(t, source)
	readDone := beginPlanReviewRead(router)
	assertPlanReviewReadBlocked(t, readDone)

	router.EnableReview()
	writePlanReviewInput(t, input, "\x1b[A")
	if got := nextPlanReviewKey(t, router); got != planReviewKeyUp {
		t.Fatalf("NextReviewKey() = %d, want Up", got)
	}
	assertPlanReviewReadBlocked(t, readDone)

	router.DisableReview()
	writePlanReviewInput(t, input, "line")
	result := awaitPlanReviewRead(t, readDone)
	if result.err != nil {
		t.Fatalf("Read() error = %v", result.err)
	}
	if string(result.data) != "line" {
		t.Fatalf("Read() = %q, want line", result.data)
	}
}

func TestPlanReviewInputRouterDecodesFragmentedNavigationSequences(t *testing.T) {
	for _, test := range []struct {
		name   string
		pieces []string
		want   planReviewKey
	}{
		{name: "CSI up", pieces: []string{"\x1b", "[", "A"}, want: planReviewKeyUp},
		{name: "CSI down", pieces: []string{"\x1b", "[", "B"}, want: planReviewKeyDown},
		{name: "SS3 up", pieces: []string{"\x1b", "O", "A"}, want: planReviewKeyUp},
		{name: "SS3 down", pieces: []string{"\x1b", "O", "B"}, want: planReviewKeyDown},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, input := io.Pipe()
			t.Cleanup(func() {
				_ = input.Close()
				_ = source.Close()
			})
			router := mustPlanReviewInputRouter(t, source)
			router.EnableReview()
			for _, piece := range test.pieces {
				writePlanReviewInput(t, input, piece)
			}
			if got := nextPlanReviewKey(t, router); got != test.want {
				t.Fatalf("NextReviewKey() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestPlanReviewInputRouterDistinguishesBareEscape(t *testing.T) {
	source, input := io.Pipe()
	t.Cleanup(func() {
		_ = input.Close()
		_ = source.Close()
	})
	router := mustPlanReviewInputRouter(t, source)
	router.EnableReview()
	writePlanReviewInput(t, input, "\x1b")
	if got := nextPlanReviewKey(t, router); got != planReviewKeyEscape {
		t.Fatalf("NextReviewKey() = %d, want Escape", got)
	}
}

func TestPlanReviewInputRouterDoesNotLeakPartialSequenceAcrossDisable(t *testing.T) {
	source, input := io.Pipe()
	t.Cleanup(func() {
		_ = input.Close()
		_ = source.Close()
	})
	router := mustPlanReviewInputRouter(t, source)
	router.EnableReview()
	writePlanReviewInput(t, input, "\x1b[")
	waitForPlanReviewDecoderState(t, router, planReviewDecoderCSI)
	router.DisableReview()

	readDone := beginPlanReviewRead(router)
	writePlanReviewInput(t, input, "Aordinary")
	result := awaitPlanReviewRead(t, readDone)
	if result.err != nil {
		t.Fatalf("Read() error = %v", result.err)
	}
	if string(result.data) != "ordinary" {
		t.Fatalf("Read() = %q, want partial sequence suppressed", result.data)
	}
}

func TestPlanReviewInputRouterDiscardsTimedOutPartialSequence(t *testing.T) {
	source, input := io.Pipe()
	t.Cleanup(func() {
		_ = input.Close()
		_ = source.Close()
	})
	router := mustPlanReviewInputRouter(t, source)
	router.EnableReview()
	writePlanReviewInput(t, input, "\x1b[")
	time.Sleep(2 * planReviewEscapeTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), planReviewEscapeTimeout)
	defer cancel()
	_, err := router.NextReviewKey(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("NextReviewKey() error = %v, want deadline with no leaked key", err)
	}
	router.DisableReview()
	readDone := beginPlanReviewRead(router)
	writePlanReviewInput(t, input, "ordinary")
	result := awaitPlanReviewRead(t, readDone)
	if result.err != nil || string(result.data) != "ordinary" {
		t.Fatalf("Read() = (%q, %v), want ordinary bytes", result.data, result.err)
	}
}

func TestPlanReviewInputRouterPropagatesUnderlyingContextCancellation(t *testing.T) {
	sourceContext, cancelSource := context.WithCancel(context.Background())
	router := mustPlanReviewInputRouter(t, planReviewContextReader{ctx: sourceContext})
	router.EnableReview()
	readDone := beginPlanReviewRead(router)
	keyDone := make(chan error, 1)
	go func() {
		_, err := router.NextReviewKey(context.Background())
		keyDone <- err
	}()
	cancelSource()

	readResult := awaitPlanReviewRead(t, readDone)
	if !errors.Is(readResult.err, context.Canceled) {
		t.Fatalf("Read() error = %v, want context canceled", readResult.err)
	}
	select {
	case err := <-keyDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("NextReviewKey() error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("NextReviewKey() did not receive source cancellation")
	}
}

func TestPlanReviewInputRouterHonorsReviewConsumerContext(t *testing.T) {
	source, input := io.Pipe()
	t.Cleanup(func() {
		_ = input.Close()
		_ = source.Close()
	})
	router := mustPlanReviewInputRouter(t, source)
	router.EnableReview()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := router.NextReviewKey(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("NextReviewKey() error = %v, want context canceled", err)
	}
	if !router.ReviewEnabled() {
		t.Fatal("review consumer cancellation disabled routing")
	}
}

func TestPlanReviewInputRouterDisableStopsForwarderAndReenableUsesFreshOne(t *testing.T) {
	source, input := io.Pipe()
	t.Cleanup(func() {
		_ = input.Close()
		_ = source.Close()
	})
	router := mustPlanReviewInputRouter(t, source)
	router.EnableReview()
	oldForwarder := make(chan error, 1)
	go func() {
		_, err := router.NextReviewKey(context.Background())
		oldForwarder <- err
	}()
	time.Sleep(planReviewEscapeTimeout)
	router.DisableReview()
	router.EnableReview()
	writePlanReviewInput(t, input, " ")
	if got := nextPlanReviewKey(t, router); got != planReviewKeyToggle {
		t.Fatalf("fresh forwarder key = %d, want Toggle", got)
	}
	select {
	case err := <-oldForwarder:
		if !errors.Is(err, errPlanReviewInputInactive) {
			t.Fatalf("old forwarder error = %v, want inactive", err)
		}
	case <-time.After(time.Second):
		t.Fatal("old forwarder remained active across a new review generation")
	}
}

func TestPlanReviewInputRouterRepeatedEnableKeepsCurrentForwarder(t *testing.T) {
	source, input := io.Pipe()
	t.Cleanup(func() {
		_ = input.Close()
		_ = source.Close()
	})
	router := mustPlanReviewInputRouter(t, source)
	router.EnableReview()
	forwarder := make(chan planReviewReadResult, 1)
	go func() {
		key, err := router.NextReviewKey(context.Background())
		forwarder <- planReviewReadResult{data: []byte{byte(key)}, err: err}
	}()
	time.Sleep(planReviewEscapeTimeout)
	router.EnableReview()
	writePlanReviewInput(t, input, " ")

	result := awaitPlanReviewRead(t, forwarder)
	if result.err != nil {
		t.Fatalf("forwarder error = %v", result.err)
	}
	if len(result.data) != 1 || planReviewKey(result.data[0]) != planReviewKeyToggle {
		t.Fatalf("forwarder key = %v, want Toggle", result.data)
	}
}

func TestPlanReviewInputRouterBoundsUnreadPassthroughBytes(t *testing.T) {
	t.Parallel()

	want := strings.Repeat("ordinary-input-", 512)
	router := mustPlanReviewInputRouter(t, strings.NewReader(want))
	router.start()
	waitForPlanReviewBufferedInput(t, router, false)
	time.Sleep(2 * planReviewEscapeTimeout)
	router.mu.Lock()
	buffered := len(router.lineBytes)
	router.mu.Unlock()
	if buffered < 1 || buffered > 64 {
		t.Fatalf("unread passthrough buffer = %d bytes, want 1..64", buffered)
	}
	actual, err := io.ReadAll(router)
	if err != nil || string(actual) != want {
		t.Fatalf("bounded passthrough changed input: bytes=%d error=%v", len(actual), err)
	}
}

func TestPlanReviewInputRouterBoundsUnconsumedReviewKeys(t *testing.T) {
	t.Parallel()

	const keyCount = 4096
	router := mustPlanReviewInputRouter(t, strings.NewReader(strings.Repeat(" ", keyCount)))
	router.EnableReview()
	waitForPlanReviewBufferedInput(t, router, true)
	time.Sleep(2 * planReviewEscapeTimeout)
	router.mu.Lock()
	buffered := len(router.reviewKeys)
	router.mu.Unlock()
	if buffered < 1 || buffered > 64 {
		t.Fatalf("unconsumed review-key buffer = %d keys, want 1..64", buffered)
	}
	for index := 0; index < keyCount; index++ {
		if got := nextPlanReviewKey(t, router); got != planReviewKeyToggle {
			t.Fatalf("review key %d = %d, want Toggle", index, got)
		}
	}
	_, err := router.NextReviewKey(context.Background())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("review input terminal error = %v, want EOF", err)
	}
}
